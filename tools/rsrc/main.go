// Writes rsrc_windows_amd64.syso, the object file the Go linker picks up out of
// the module root and copies into the executable's .rsrc section.  Two things go
// in it, and both are things Windows asks the file about rather than things the
// program asks for:
//
//	the icon     what Explorer draws for the file, what the taskbar draws for
//	             the window, and what `ui.Run` asks for by id
//	the version  the Details tab of the Properties dialog, and what anything
//	             calling GetFileVersionInfo gets back
//
// Written here rather than taken from a tool because the alternative is a build
// dependency: go-winres and rsrc both do this, both would have to be resolved
// before the launcher compiles, and CI builds this repository with nothing
// installed but Go on purpose.  What it emits is a COFF object with one section
// and a handful of relocations, which is small enough to own.
//
//	go run ./tools/rsrc              # writes ../../rsrc_windows_amd64.syso
//	go run ./tools/rsrc -o out.syso
//
// The .ico is checked in and comes from tools/rsrc/icon.py.  The version comes
// from .version, the same file the payload is stamped with, so the number in the
// Properties dialog and the number under the title on the page cannot disagree.
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"
)

// The resource types written here.  RT_ICON is one image.  RT_GROUP_ICON is
// the directory over them, and it is the one a caller asks for by id.  Windows
// then picks whichever image fits the size it was asked to draw.  RT_VERSION is the
// version block.
const (
	typeIcon      = 3
	typeVersion   = 16
	typeGroupIcon = 14
)

// GroupID is the id of the icon group, and the number ui.Run passes as IconId.
// One icon, so 1.  The images under it get 1..n of their own, in the RT_ICON
// space, which is a different space entirely.  VersionID is 1 for the same
// reason, in a third space: GetFileVersionInfo looks for exactly that one.
const (
	GroupID   = 1
	VersionID = 1
)

// What the Properties dialog says.  Nothing in here is a fact about the machine
// that built the file, deliberately: two builds of one commit produce the same
// bytes.
const (
	company     = "ancaria.dev"
	productName = "Sacred Mod Loader"
	describe    = "Mod loader for Sacred Gold"
	fileName    = "Sacred Mod Loader.exe"
	copyright   = "© 2026 MairwunNx (Pavel Erokhin). Licensed under the MIT License."
	comments    = "Installs the loader into the game folder and starts it beside " +
		"the game. The game’s own files are not modified."
)

func main() {
	source := flag.String("i", "", "ICO input file (default: tools/rsrc/sacred.ico beside this program)")
	target := flag.String("o", "", "SYSO output file (default: rsrc_windows_amd64.syso at the module root)")
	release := flag.String("v", "", "version to stamp (default: .version at the module root)")
	flag.Parse()

	root, err := moduleRoot()
	if err != nil {
		fail(err)
	}
	if *source == "" {
		*source = filepath.Join(root, "tools", "rsrc", "sacred.ico")
	}
	if *target == "" {
		*target = filepath.Join(root, "rsrc_windows_amd64.syso")
	}
	if *release == "" {
		if *release, err = readVersion(root); err != nil {
			fail(err)
		}
	}

	data, err := os.ReadFile(*source)
	if err != nil {
		fail(err)
	}
	images, err := readIcon(data)
	if err != nil {
		fail(fmt.Errorf("%s: %w", *source, err))
	}
	object, err := coff(images, version(*release))
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(*target, object, 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("%s  %d image(s) from %s, version %s\n",
		*target, len(images), filepath.Base(*source), *release)
}

// readVersion is the same .version tools/build.ps1 stamps the payload with, so
// the Properties dialog and the line under the title on the page are one file
// apart rather than two numbers somebody has to keep in step.
func readVersion(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, ".version"))
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", errors.New(".version is empty")
	}
	return text, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "Icon:", err)
	os.Exit(1)
}

// moduleRoot walks up from the working directory to the go.mod, so the program
// writes the .syso where the linker looks for it whichever directory it was
// started from.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("No go.mod found above the working directory")
		}
		dir = parent
	}
}

// --- the .ico ------------------------------------------------------------

// image is one entry of an .ico: the sixteen bytes describing it and the bytes
// of the image itself, which are copied into the executable untouched.
type image struct {
	head [16]byte
	data []byte
}

// readIcon parses an ICONDIR and its entries.  Nothing is decoded: a .ico holds
// either a DIB or a whole PNG per entry and Windows reads both, so the bytes go
// across as they are.
func readIcon(data []byte) ([]image, error) {
	if len(data) < 6 {
		return nil, errors.New("Invalid icon: file is too short for a header")
	}
	if binary.LittleEndian.Uint16(data[0:]) != 0 || binary.LittleEndian.Uint16(data[2:]) != 1 {
		return nil, errors.New("Invalid icon: header is not ICONDIR")
	}
	count := int(binary.LittleEndian.Uint16(data[4:]))
	if count == 0 {
		return nil, errors.New("The icon contains no images")
	}
	if len(data) < 6+count*16 {
		return nil, errors.New("The icon directory is truncated")
	}

	images := make([]image, 0, count)
	for i := range count {
		entry := data[6+i*16:]
		size := binary.LittleEndian.Uint32(entry[8:])
		offset := binary.LittleEndian.Uint32(entry[12:])
		end := uint64(offset) + uint64(size)
		if end > uint64(len(data)) {
			return nil, fmt.Errorf("Image %d extends past the end of the file", i+1)
		}
		var one image
		copy(one.head[:], entry[:16])
		one.data = data[offset:end]
		images = append(images, one)
	}
	return images, nil
}

// group is the RT_GROUP_ICON resource: the same directory the .ico carries,
// with each entry's file offset replaced by the RT_ICON id holding it.  Fourteen
// bytes per entry rather than sixteen, which is the only reason it cannot be
// copied straight out of the file.
func group(images []image) []byte {
	out := new(bytes.Buffer)
	_ = binary.Write(out, binary.LittleEndian, uint16(0))
	_ = binary.Write(out, binary.LittleEndian, uint16(1))
	_ = binary.Write(out, binary.LittleEndian, uint16(len(images)))
	for i, one := range images {
		// The first eight bytes are the same in both structs (width, height,
		// colours, planes, bit count) and everything after them differs. An
		// ICONDIRENTRY is sixteen bytes and ends in a file offset. A
		// GRPICONDIRENTRY is fourteen and ends in a resource id.
		out.Write(one.head[:8])
		_ = binary.Write(out, binary.LittleEndian, uint32(len(one.data))) // dwBytesInRes
		_ = binary.Write(out, binary.LittleEndian, uint16(i+1))           // nId
	}
	return out.Bytes()
}

// --- the version block ---------------------------------------------------

// version builds VS_VERSIONINFO: a tree of length-prefixed nodes, each one a
// header, a UTF-16 key, and either binary bytes or child nodes, with every part
// of it padded to a four-byte boundary.  Explorer reads it for the Details tab,
// and GetFileVersionInfo, which is what `game.Version` calls on the game's own
// executable, reads the fixed block at the top of it.
//
// The fixed block carries the numbers and the string table carries the text, and
// the format lets the two disagree. That is exactly the trap `game/exe.go`
// documents about the game itself: the HD wrapper spells its FileVersion "2.28"
// while its fixed block says 2.0.2.118. Both halves come from one argument here
// for that reason.
func version(release string) []byte {
	numbers := quad(release)

	fixed := new(bytes.Buffer)
	write := func(values ...uint32) {
		for _, value := range values {
			_ = binary.Write(fixed, binary.LittleEndian, value)
		}
	}
	write(0xFEEF04BD, 0x00010000)                               // dwSignature, dwStrucVersion
	write(numbers[0]<<16|numbers[1], numbers[2]<<16|numbers[3]) // dwFileVersion MS, LS
	write(numbers[0]<<16|numbers[1], numbers[2]<<16|numbers[3]) // dwProductVersion MS, LS
	write(0x3F, 0)                                              // dwFileFlagsMask, dwFileFlags
	write(4, 1, 0)                                              // VOS__WINDOWS32, VFT_APP, subtype
	write(0, 0)                                                 // dwFileDate MS, LS

	// The language and code page the strings below are in, spelled twice: as the
	// name of the table, in hex, and as the binary value under VarFileInfo.
	// 0x0409 is en-US and 0x04B0 is 1200, which is UTF-16, and the strings
	// really are UTF-16, so anything else here is a lie about the bytes.
	const language, codePage = 0x0409, 0x04B0
	table := fmt.Sprintf("%04X%04X", language, codePage)

	// Ordered, because the Details tab lists them in the order it finds them and
	// a map would shuffle them from one build to the next.
	values := [][2]string{
		{"CompanyName", company},
		{"FileDescription", describe},
		{"FileVersion", release},
		{"InternalName", productName},
		{"LegalCopyright", copyright},
		{"OriginalFilename", fileName},
		{"ProductName", productName},
		{"ProductVersion", release},
		{"Comments", comments},
	}
	entries := make([][]byte, 0, len(values))
	for _, one := range values {
		entries = append(entries, node(one[0], text(one[1]), true))
	}

	var translation []byte
	translation = binary.LittleEndian.AppendUint16(translation, language)
	translation = binary.LittleEndian.AppendUint16(translation, codePage)

	return node("VS_VERSION_INFO", fixed.Bytes(), false,
		node("StringFileInfo", nil, true, node(table, nil, true, entries...)),
		node("VarFileInfo", nil, true, node("Translation", translation, false)))
}

// node is one length-prefixed record.  wValueLength counts characters for a text
// value and bytes for a binary one, which is the single field in this format
// that means two different things depending on wType.
//
// The header goes down before anything else because every pad in this format is
// measured from the start of the record, and the header is three words of it.
// Aligning what comes after the key on its own is off by exactly those six
// bytes: `VS_VERSION_INFO` plus its NUL is thirty-two, which needs two bytes of
// padding at offset 38 and none at offset 32, so the fixed block lands two
// bytes early, its signature reads as 0x0000FEEF, and Windows answers every
// question about the file with an empty string.
//
// wLength does not count any padding after the record.  The parent adds that
// before whatever follows, which is why nothing here writes a trailing pad.
func node(key string, value []byte, isText bool, children ...[]byte) []byte {
	length := len(value)
	kind := uint16(0)
	if isText {
		// Characters, not bytes, and the terminating NUL counts as one.
		length /= 2
		kind = 1
	}

	out := new(bytes.Buffer)
	_ = binary.Write(out, binary.LittleEndian, uint16(0)) // wLength, filled in below
	_ = binary.Write(out, binary.LittleEndian, uint16(length))
	_ = binary.Write(out, binary.LittleEndian, kind)
	out.Write(text(key))
	pad(out, 4)
	out.Write(value)
	for _, child := range children {
		pad(out, 4)
		out.Write(child)
	}

	written := out.Bytes()
	binary.LittleEndian.PutUint16(written, uint16(len(written)))
	return written
}

// text is a NUL-terminated UTF-16 string, which is what every key and every
// text value in this format is.
func text(value string) []byte {
	out := make([]byte, 0, len(value)*2+2)
	for _, unit := range utf16.Encode([]rune(value)) {
		out = binary.LittleEndian.AppendUint16(out, unit)
	}
	return binary.LittleEndian.AppendUint16(out, 0)
}

func pad(buffer *bytes.Buffer, to int) {
	for buffer.Len()%to != 0 {
		buffer.WriteByte(0)
	}
}

// quad turns 0.1.20 into the four numbers the fixed block wants, padding with
// zeroes.  Each part is read up to its first non-digit rather than trimmed of
// them, because `20-rc1` ends in a digit and trimming leaves the whole thing --
// which then parses as nothing and puts a zero where the 20 belonged. A
// qualifier is something the string half can carry and this half cannot.
func quad(release string) [4]uint32 {
	var out [4]uint32
	for i, part := range strings.SplitN(release, ".", 5) {
		if i > 3 {
			break
		}
		digits := part
		if at := strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }); at >= 0 {
			digits = part[:at]
		}
		value, err := strconv.ParseUint(digits, 10, 16)
		if err != nil {
			continue
		}
		out[i] = uint32(value)
	}
	return out
}

// --- the resource tree ---------------------------------------------------

// blob is one leaf: a type, an id, its bytes, and where the bytes and their
// descriptor ended up once the tree was laid out.
type blob struct {
	kind  uint32 // RT_ICON, RT_GROUP_ICON or RT_VERSION
	id    uint32
	bytes []byte

	entry uint32 // offset of the IMAGE_RESOURCE_DATA_ENTRY
	at    uint32 // offset of the bytes
}

const (
	dirSize   = 16 // IMAGE_RESOURCE_DIRECTORY
	dirEntry  = 8  // IMAGE_RESOURCE_DIRECTORY_ENTRY
	dataEntry = 16 // IMAGE_RESOURCE_DATA_ENTRY
	subdir    = 0x80000000
)

// resources lays the three-level tree Windows expects (type, then name, then
// language) and returns the section's bytes with the offsets of every
// OffsetToData field, which are the ones the linker has to turn into RVAs.
//
// A loop over whatever it is given rather than three types with their offsets
// worked out by hand. Those offsets are what the resource loader
// binary-searches, and one more resource added to a hand-laid tree is a tree
// that points into the middle of itself.
func resources(blobs []blob) (section []byte, fixups []uint32) {
	// Grouped by type, in the order they arrive.  The caller keeps them sorted by
	// type number because every level of the tree is binary-searched.
	var kinds []uint32
	byKind := map[uint32][]int{}
	for i, one := range blobs {
		if _, seen := byKind[one.kind]; !seen {
			kinds = append(kinds, one.kind)
		}
		byKind[one.kind] = append(byKind[one.kind], i)
	}

	// Where every directory and every leaf lands, in the order the second pass
	// writes them. Nothing is emitted here: the two passes getting out of step
	// is the whole failure mode of this function.
	offset := uint32(dirSize + len(kinds)*dirEntry)
	typeDir := map[uint32]uint32{}
	for _, kind := range kinds {
		typeDir[kind] = offset
		offset += uint32(dirSize + len(byKind[kind])*dirEntry)
	}
	langDir := make([]uint32, len(blobs))
	for _, kind := range kinds {
		for _, i := range byKind[kind] {
			langDir[i] = offset
			offset += uint32(dirSize + dirEntry)
		}
	}
	for i := range blobs {
		blobs[i].entry = offset
		offset += dataEntry
	}
	for i := range blobs {
		offset = align(offset, 8)
		blobs[i].at = offset
		offset += uint32(len(blobs[i].bytes))
	}

	out := make([]byte, 0, offset)
	put := func(values ...uint32) {
		for _, value := range values {
			out = binary.LittleEndian.AppendUint32(out, value)
		}
	}
	// header writes an IMAGE_RESOURCE_DIRECTORY with `count` id entries and no
	// named ones. Nothing here has a string name.
	header := func(count int) {
		put(0, 0, 0)
		out = binary.LittleEndian.AppendUint16(out, 0)
		out = binary.LittleEndian.AppendUint16(out, uint16(count))
	}

	// Level one: the types.
	header(len(kinds))
	for _, kind := range kinds {
		put(kind, typeDir[kind]|subdir)
	}
	// Level two: the ids under each type.
	for _, kind := range kinds {
		header(len(byKind[kind]))
		for _, i := range byKind[kind] {
			put(blobs[i].id, langDir[i]|subdir)
		}
	}
	// Level three: one language each, and it is the neutral one, which is what
	// FindResource falls back to whatever language the caller asked for.
	for _, kind := range kinds {
		for _, i := range byKind[kind] {
			header(1)
			put(0, blobs[i].entry)
		}
	}

	// The leaves. OffsetToData is an RVA in a linked image and a section-relative
	// offset here, which is what the relocation is for.
	for _, one := range blobs {
		fixups = append(fixups, uint32(len(out)))
		put(one.at, uint32(len(one.bytes)), 0, 0)
	}
	for _, one := range blobs {
		for uint32(len(out)) < one.at {
			out = append(out, 0)
		}
		out = append(out, one.bytes...)
	}
	return out, fixups
}

func align(offset, to uint32) uint32 {
	if remainder := offset % to; remainder != 0 {
		return offset + to - remainder
	}
	return offset
}

// --- the object file -----------------------------------------------------

const (
	machineAMD64 = 0x8664
	// IMAGE_SCN_CNT_INITIALIZED_DATA | IMAGE_SCN_MEM_READ
	sectionFlags = 0x40000040
	// IMAGE_REL_AMD64_ADDR32NB: the 32-bit RVA of the target, which is exactly
	// what every OffsetToData field has to become.
	relocAddr32NB  = 0x0003
	symClassStatic = 3
)

// coff writes a COFF object holding one .rsrc section, its relocations, and the
// single static symbol they are all relative to.
func coff(images []image, versionInfo []byte) ([]byte, error) {
	// Sorted by type number: RT_ICON 3, RT_GROUP_ICON 14, RT_VERSION 16. Every
	// level of the tree is binary-searched, so an out-of-order entry is a
	// resource Windows quietly cannot find rather than one it complains about.
	blobs := make([]blob, 0, len(images)+2)
	for i, one := range images {
		blobs = append(blobs, blob{kind: typeIcon, id: uint32(i + 1), bytes: one.data})
	}
	blobs = append(blobs, blob{kind: typeGroupIcon, id: GroupID, bytes: group(images)})
	blobs = append(blobs, blob{kind: typeVersion, id: VersionID, bytes: versionInfo})

	section, fixups := resources(blobs)
	if len(fixups) > 0xffff {
		return nil, errors.New("Too many resources for one object file")
	}

	const header = 20
	const sectionHeader = 40
	dataAt := uint32(header + sectionHeader)
	relocAt := dataAt + uint32(len(section))
	symbolsAt := relocAt + uint32(len(fixups)*10)

	out := new(bytes.Buffer)
	write := func(values ...any) {
		for _, value := range values {
			_ = binary.Write(out, binary.LittleEndian, value)
		}
	}

	// IMAGE_FILE_HEADER
	write(uint16(machineAMD64), uint16(1), uint32(0), symbolsAt, uint32(2), uint16(0), uint16(0))

	// IMAGE_SECTION_HEADER
	out.Write([]byte(".rsrc\x00\x00\x00"))
	write(uint32(0), uint32(0), uint32(len(section)), dataAt, relocAt,
		uint32(0), uint16(len(fixups)), uint16(0), uint32(sectionFlags))

	out.Write(section)

	for _, at := range fixups {
		write(at, uint32(0), uint16(relocAddr32NB))
	}

	// One symbol for the section, and the auxiliary record that always follows a
	// static section symbol. Every relocation above points at index 0.
	out.Write([]byte(".rsrc\x00\x00\x00"))
	write(uint32(0), uint16(1), uint16(0), uint8(symClassStatic), uint8(1))
	write(uint32(len(section)), uint16(len(fixups)), uint16(0), uint32(0), uint16(0), uint8(0))
	out.Write(make([]byte, 3))

	// An empty string table is still four bytes saying so.
	write(uint32(4))
	return out.Bytes(), nil
}
