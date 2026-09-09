package main

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

// Both failures this file exists for were silent. A version block with its
// padding measured from the wrong place still links, still shows up in the
// resource tree at the right type and id, and answers every question Windows
// asks it with an empty string.  A group icon with the wrong stride does the same
// and leaves the file drawn with the default icon. Neither produces an error
// anywhere, so the check has to be a reader.

func TestQuad(t *testing.T) {
	cases := map[string][4]uint32{
		"0.1.20":     {0, 1, 20, 0},
		"1.2.3.4":    {1, 2, 3, 4},
		"2":          {2, 0, 0, 0},
		"0.1.20-rc1": {0, 1, 20, 0},
		"1.2.3.4.5":  {1, 2, 3, 4},
		"":           {0, 0, 0, 0},
		"x.y":        {0, 0, 0, 0},
	}
	for release, want := range cases {
		if got := quad(release); got != want {
			t.Errorf("quad(%q) = %v, wanted %v", release, got, want)
		}
	}
}

// The fixed block is what GetFileVersionInfo reads, and it is found by walking
// past the key and its padding rather than at a fixed offset, so this is the
// one place the padding rule has to be right.
func TestVersionBlockCarriesTheFixedInfo(t *testing.T) {
	block := version("0.1.20")

	length, valueLength, kind, key, value := record(t, block)
	if length != len(block) {
		t.Errorf("wLength is %d and the block is %d bytes", length, len(block))
	}
	if key != "VS_VERSION_INFO" {
		t.Fatalf("the root record is called %q", key)
	}
	if kind != 0 || valueLength != 52 {
		t.Fatalf("the root value is type %d, %d bytes; wanted binary and 52", kind, valueLength)
	}
	if signature := binary.LittleEndian.Uint32(value); signature != 0xFEEF04BD {
		t.Fatalf("the fixed block signature is %#08x, not VS_FIXEDFILEINFO's %#08x",
			signature, 0xFEEF04BD)
	}
	// dwFileVersionMS and LS: 0.1 then 20.0.
	if ms := binary.LittleEndian.Uint32(value[8:]); ms != 0<<16|1 {
		t.Errorf("dwFileVersionMS is %#08x", ms)
	}
	if ls := binary.LittleEndian.Uint32(value[12:]); ls != 20<<16|0 {
		t.Errorf("dwFileVersionLS is %#08x", ls)
	}
}

// And the strings are what the Details tab lists.
func TestVersionBlockCarriesTheStrings(t *testing.T) {
	block := version("0.1.20")
	for _, want := range []string{
		"VS_VERSION_INFO", "StringFileInfo", "040904B0", "VarFileInfo", "Translation",
		"CompanyName", company, "OriginalFilename", fileName, "FileVersion", "0.1.20",
	} {
		if !strings.Contains(string(block), utf16le(want)) {
			t.Errorf("the block does not carry %q", want)
		}
	}
}

// record reads one node the way the format is defined, padding included, which
// is the whole point of reading it here rather than trusting the writer.
func record(t *testing.T, block []byte) (length, valueLength, kind int, key string, value []byte) {
	t.Helper()
	if len(block) < 6 {
		t.Fatal("shorter than a header")
	}
	length = int(binary.LittleEndian.Uint16(block))
	valueLength = int(binary.LittleEndian.Uint16(block[2:]))
	kind = int(binary.LittleEndian.Uint16(block[4:]))

	at := 6
	var units []uint16
	for at+2 <= len(block) {
		unit := binary.LittleEndian.Uint16(block[at:])
		at += 2
		if unit == 0 {
			break
		}
		units = append(units, unit)
	}
	key = string(utf16.Decode(units))

	// Padding to the next four-byte boundary, measured from the start of the
	// record, which is what the writer got wrong the first time.
	at = (at + 3) & ^3
	size := valueLength
	if kind == 1 {
		size *= 2
	}
	if at+size > len(block) {
		t.Fatalf("the value runs past the end of the record")
	}
	return length, valueLength, kind, key, block[at : at+size]
}

func utf16le(value string) string {
	var out []byte
	for _, unit := range utf16.Encode([]rune(value)) {
		out = binary.LittleEndian.AppendUint16(out, unit)
	}
	return string(out)
}

// A GRPICONDIRENTRY is fourteen bytes to an ICONDIRENTRY's sixteen, and only
// their first eight are the same. Copying the wrong number of them is a file
// Explorer draws with the default icon and complains about nowhere.
func TestGroupEntriesAreFourteenBytesEach(t *testing.T) {
	images := []image{{data: []byte("first")}, {data: []byte("second")}}
	images[0].head = [16]byte{32, 32, 0, 0, 1, 0, 32, 0, 9, 9, 9, 9, 7, 7, 7, 7}
	images[1].head = [16]byte{16, 16, 0, 0, 1, 0, 32, 0, 8, 8, 8, 8, 6, 6, 6, 6}

	directory := group(images)
	if want := 6 + len(images)*14; len(directory) != want {
		t.Fatalf("the directory is %d bytes, wanted %d", len(directory), want)
	}
	if binary.LittleEndian.Uint16(directory[2:]) != 1 {
		t.Error("the type word does not say icon")
	}
	if count := binary.LittleEndian.Uint16(directory[4:]); int(count) != len(images) {
		t.Errorf("the directory counts %d images", count)
	}
	for i, one := range images {
		entry := directory[6+i*14:]
		if entry[0] != one.head[0] || entry[1] != one.head[1] {
			t.Errorf("entry %d is %dx%d", i, entry[0], entry[1])
		}
		// The two fields that are this struct's own rather than the file's.
		if size := binary.LittleEndian.Uint32(entry[8:]); int(size) != len(one.data) {
			t.Errorf("entry %d says %d bytes, the image is %d", i, size, len(one.data))
		}
		if id := binary.LittleEndian.Uint16(entry[12:]); int(id) != i+1 {
			t.Errorf("entry %d names RT_ICON %d", i, id)
		}
	}
}
