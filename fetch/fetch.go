// Package fetch is one HTTP download, done the same way everywhere.
//
// Two things in this launcher pull bytes off the internet: a JDK, which is
// hundreds of megabytes and has to show a bar, and a mod with its registry
// index and its icon, which are small and also have to show a bar. They agree
// on progress reporting, on hashing, and on refusing to leave half a file
// behind, so they agree in one place.
package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Transport is what every request in this launcher goes through.
//
// A variable rather than the default because the tests around the mod registry
// have to serve real HTTPS from a certificate nobody signed, and the alternative
// is a registry that is only ever exercised against a fake of itself.
var Transport http.RoundTripper = http.DefaultTransport

// Bytes reads a small resource into memory.
//
// limit is a cap on what is read rather than an expectation: an index or an
// icon is kilobytes, and a URL that answers with a gigabyte is a URL that must
// not be able to fill the machine's memory before anybody notices.
func Bytes(url string, head http.Header, limit int64) ([]byte, error) {
	client := &http.Client{Transport: Transport, Timeout: 20 * time.Second}
	response, err := send(context.Background(), client, url, head)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("The download from %s stopped early: %w", url, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("The response from %s exceeds the %d-byte limit", url, limit)
	}
	return data, nil
}

// File downloads url into path, hashing on the way, and returns the hex SHA-256
// of what arrived.
//
// Nothing partial survives a failure: the caller decides what the hash was
// supposed to be, and a half file that looks whole is how a loader ends up
// running bytes nobody checked.
//
// max stops a download that has already gone past the size it was advertised
// at. The hash would catch the same lie afterwards, which is no use to somebody
// whose disk filled up while it was being told.
func File(url string, head http.Header, path string, max int64, progress func(done, total int64)) (string, error) {
	return FileContext(context.Background(), url, head, path, max, progress)
}

// FileContext is File that stops when ctx is cancelled, for the one download a
// player can abort.  What was written so far is deleted the same way a
// dropped connection's is.
func FileContext(ctx context.Context, url string, head http.Header, path string, max int64, progress func(done, total int64)) (string, error) {
	// No overall timeout. A transfer here can be minutes long by design, and a
	// deadline meant for an index would abort it halfway every time.
	response, err := send(ctx, &http.Client{Transport: Transport}, url, head)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	file, err := os.Create(path)
	if err != nil {
		return "", err
	}
	var body io.Reader = response.Body
	if max > 0 {
		body = io.LimitReader(body, max)
	}
	digest := sha256.New()
	err = Copy(io.MultiWriter(file, digest), body, response.ContentLength, progress)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("The download from %s stopped early: %w", url, err)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// Copy is io.Copy with a report roughly four times a second: often enough that
// the bar moves, rarely enough that the page is not redrawing on every 32 KB
// block.
func Copy(into io.Writer, from io.Reader, total int64, progress func(done, total int64)) error {
	buffer := make([]byte, 256<<10)
	var done int64
	last := time.Now()
	for {
		read, err := from.Read(buffer)
		if read > 0 {
			if _, writeErr := into.Write(buffer[:read]); writeErr != nil {
				return writeErr
			}
			done += int64(read)
			if progress != nil && time.Since(last) > 250*time.Millisecond {
				progress(done, total)
				last = time.Now()
			}
		}
		if err == io.EOF {
			if progress != nil {
				progress(done, total)
			}
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func send(ctx context.Context, client *http.Client, url string, head http.Header) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for name, values := range head {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Could not reach %s: %w", url, err)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, &Status{URL: url, Code: response.StatusCode, Text: response.Status}
	}
	return response, nil
}

// Status is an answer that was not 200.  A type rather than a string because
// the difference between 404 and 401 decides what a player is told: one is the
// wrong address, the other is a repository that wants a token.
type Status struct {
	URL  string
	Code int
	Text string
}

func (s *Status) Error() string {
	return s.URL + " returned " + s.Text
}
