// Package pin is the notation a mod uses to say which loader it runs on.
//
// A descriptor carries two of these, `api` and `loader`, and both are ranges
// rather than numbers, which is the whole point: a mod that works on API 1 and
// on API 2 has no way to say so with a number, and a mod that needs a fix
// released in 0.2.0 has no way to say that either. The syntax is Maven's, the
// one NeoForge writes in a `mods.toml`, so it is a notation somebody has
// probably already read:
//
//	[1,2)        1 or newer, below 2 -- one major, the usual thing to write
//	[1.0,)       1.0 or newer, no upper end
//	(,1.5]       anything up to and including 1.5
//	[1.2]        that version and nothing else
//	[1,2),[3,4)  either of those, and nothing in between
//	1            the same as [1]: that version and nothing else
//
// Square brackets include the end, round ones exclude it, and an end left blank
// is no end at all.
//
// # One deliberate difference from Maven
//
// A bare `1` in Maven is a *soft* requirement -- a recommendation, satisfied by
// any version at all. That answer is useless here and dangerous: the question
// this package exists to answer is whether a mod will run, and "any" is the one
// reply that is never true. A bare version therefore means exactly that version,
// which is also what every `api = "1"` written before ranges existed meant.
//
// # Versions
//
// Dotted numbers with an optional qualifier after the first `-` or `+`:
// `1`, `0.1.20`, `2.0.2.118`, `1.0.0-rc1`, `25.0.4.1+1`. Missing parts count as
// zero, so `1` and `1.0.0` are the same version. A qualifier sorts *before* the
// version without one, because `1.0.0-rc1` comes before `1.0.0`; two qualifiers
// are compared as text, which is not clever and does not pretend to be.
//
// Anything that is not that shape is unknown rather than zero, and an unknown
// version satisfies nothing. Refusing to guess is the point: a version nobody
// can order is a compatibility answer nobody should trust.
package pin

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Version is one release, in a form that can be ordered.
type Version struct {
	parts     []int
	qualifier string
	text      string
}

// Parse reads a version.  An unreadable one comes back unknown rather than as
// an error: every caller here has the original string to print and nothing
// useful to do with a second description of it.
func Parse(text string) Version {
	text = strings.Trim(strings.TrimSpace(text), `"`)
	if text == "" {
		return Version{}
	}
	head, qualifier := text, ""
	if at := strings.IndexAny(text, "-+"); at >= 0 {
		head, qualifier = text[:at], text[at+1:]
	}
	if head == "" {
		return Version{}
	}
	fields := strings.Split(head, ".")
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		value, err := strconv.Atoi(field)
		if err != nil || value < 0 {
			return Version{}
		}
		parts = append(parts, value)
	}
	return Version{parts: parts, qualifier: qualifier, text: text}
}

// Known reports whether this is a version at all.
func (v Version) Known() bool { return len(v.parts) > 0 }

// String is the text it was read from, so a message says what somebody wrote
// rather than a normalised copy of it.
func (v Version) String() string { return v.text }

// Compare orders two versions: negative, zero or positive, the usual way.
func (v Version) Compare(other Version) int {
	for i := 0; i < len(v.parts) || i < len(other.parts); i++ {
		if mine, theirs := v.at(i), other.at(i); mine != theirs {
			if mine < theirs {
				return -1
			}
			return 1
		}
	}
	switch {
	case v.qualifier == other.qualifier:
		return 0
	// 1.0.0-rc1 is before 1.0.0, so having a qualifier is being earlier.
	case v.qualifier == "":
		return 1
	case other.qualifier == "":
		return -1
	}
	return strings.Compare(v.qualifier, other.qualifier)
}

// at is the part at that position, with everything past the end reading as
// zero, so 1 and 1.0.0 compare equal.
func (v Version) at(index int) int {
	if index < len(v.parts) {
		return v.parts[index]
	}
	return 0
}

// clause is one bracketed pair.  An absent end is an unknown Version, which is
// how "no end" is spelled.
type clause struct {
	low, high           Version
	lowOpen, highClosed bool
}

func (c clause) has(v Version) bool {
	if c.low.Known() {
		order := v.Compare(c.low)
		if order < 0 || (order == 0 && c.lowOpen) {
			return false
		}
	}
	if c.high.Known() {
		order := v.Compare(c.high)
		if order > 0 || (order == 0 && !c.highClosed) {
			return false
		}
	}
	return true
}

// Range is a set of versions: one or more clauses, any of which will do.
type Range struct {
	clauses []clause
	text    string
}

// ParseRange reads the notation at the top of this file.
//
// The error is meant to be shown to whoever wrote the line, so it says what was
// wrong with theirs rather than naming a production in a grammar.
func ParseRange(text string) (Range, error) {
	text = strings.Trim(strings.TrimSpace(text), `"`)
	if text == "" {
		return Range{}, nil
	}
	if !strings.ContainsAny(text[:1], "[(") {
		// A bare version. Exactly that one -- see the note at the top about why
		// this is not Maven's answer.
		version := Parse(text)
		if !version.Known() {
			return Range{}, fmt.Errorf("“%s” is neither a version nor a range. A range "+
				"looks like [1,2), and a version looks like 1.2.3", text)
		}
		return Range{
			clauses: []clause{{low: version, high: version, highClosed: true}},
			text:    text,
		}, nil
	}

	parsed := Range{text: text}
	rest := text
	for rest != "" {
		open := rest[0]
		if open != '[' && open != '(' {
			return Range{}, fmt.Errorf("“%s”: expected [ or ( at “%s”", text, rest)
		}
		end := strings.IndexAny(rest, ")]")
		if end < 0 {
			return Range{}, fmt.Errorf("“%s”: an opening bracket is not closed", text)
		}
		one, err := bounds(rest[1:end], open == '(', rest[end] == ']')
		if err != nil {
			return Range{}, fmt.Errorf("“%s”: %w", text, err)
		}
		parsed.clauses = append(parsed.clauses, one)

		rest = strings.TrimSpace(rest[end+1:])
		if rest == "" {
			break
		}
		if rest[0] != ',' {
			return Range{}, fmt.Errorf("“%s”: separate ranges with a comma", text)
		}
		rest = strings.TrimSpace(rest[1:])
		if rest == "" {
			return Range{}, fmt.Errorf("“%s”: the comma has no range after it", text)
		}
	}
	return parsed, nil
}

// bounds reads what is between one pair of brackets.
func bounds(body string, lowOpen, highClosed bool) (clause, error) {
	low, high, pair := strings.Cut(body, ",")
	if !pair {
		// A single version, which only means anything as [1.2]: (1.2) is the
		// empty set written at length, and nobody means that.
		version := Parse(body)
		if !version.Known() {
			return clause{}, fmt.Errorf("“%s” is not a version", strings.TrimSpace(body))
		}
		if lowOpen || !highClosed {
			return clause{}, fmt.Errorf("A single exact version requires square brackets: [%s]", version)
		}
		return clause{low: version, high: version, highClosed: true}, nil
	}

	one := clause{lowOpen: lowOpen, highClosed: highClosed}
	if text := strings.TrimSpace(low); text != "" {
		if one.low = Parse(text); !one.low.Known() {
			return clause{}, fmt.Errorf("“%s” is not a version", text)
		}
	}
	if text := strings.TrimSpace(high); text != "" {
		if one.high = Parse(text); !one.high.Known() {
			return clause{}, fmt.Errorf("“%s” is not a version", text)
		}
	}
	if !one.low.Known() && !one.high.Known() {
		return clause{}, errors.New("A range with no endpoints allows every version. " +
			"Remove the range instead")
	}
	if one.low.Known() && one.high.Known() && one.low.Compare(one.high) > 0 {
		return clause{}, fmt.Errorf("%s is higher than %s, so this range is empty",
			one.low, one.high)
	}
	return one, nil
}

// Any reports a range that constrains nothing, which is what an absent line
// parses to.  Kept apart from a range that happens to match: "said nothing" and
// "said yes" are different things to print.
func (r Range) Any() bool { return len(r.clauses) == 0 }

// Has reports whether a version is in the range.  An unknown version is in
// nothing, and an empty range holds everything.
func (r Range) Has(v Version) bool {
	if r.Any() {
		return true
	}
	if !v.Known() {
		return false
	}
	for _, one := range r.clauses {
		if one.has(v) {
			return true
		}
	}
	return false
}

// String is the text the range was read from.
func (r Range) String() string { return r.text }

// Allows is the whole question for a caller holding two strings: does this
// version satisfy this range?  A range that cannot be read allows nothing,
// which is the safe answer -- use ParseRange where the reason matters.
func Allows(rang, version string) bool {
	parsed, err := ParseRange(rang)
	if err != nil {
		return false
	}
	return parsed.Has(Parse(version))
}
