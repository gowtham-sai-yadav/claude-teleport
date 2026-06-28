package importer

import (
	"bytes"
	"encoding/json"
)

// pathRewriter rewrites the old machine's paths inside a transcript line.
//
// The subtlety that makes this cross-OS-correct: transcripts are JSON, so a
// Windows path like C:\Users\bob is stored on disk with escaped backslashes
// as "C:\\Users\\bob". Matching the raw path bytes would therefore miss it.
// We build the search/replace patterns from json.Marshal of each path, which
// produces exactly the escaped form that appears in the file. For Unix paths
// the escaped form equals the raw form, so this is also correct (and a no-op
// change in behaviour) for Linux<->macOS moves.
type pathRewriter struct {
	cwdCompactOld, cwdCompactNew []byte // "cwd":"<escaped>"
	cwdSpacedOld, cwdSpacedNew   []byte // "cwd": "<escaped>"
	innerOld, innerNew           []byte // <escaped> without surrounding quotes
	deep                         bool
}

func newPathRewriter(oldPath, newPath string, deep bool) *pathRewriter {
	escOld, _ := json.Marshal(oldPath) // e.g. `"C:\\Users\\bob"`
	escNew, _ := json.Marshal(newPath)
	innerOld := escOld[1 : len(escOld)-1] // strip surrounding quotes
	innerNew := escNew[1 : len(escNew)-1]

	join := func(prefix string, esc []byte) []byte {
		b := make([]byte, 0, len(prefix)+len(esc))
		b = append(b, prefix...)
		return append(b, esc...)
	}

	return &pathRewriter{
		cwdCompactOld: join(`"cwd":`, escOld),
		cwdCompactNew: join(`"cwd":`, escNew),
		cwdSpacedOld:  join(`"cwd": `, escOld),
		cwdSpacedNew:  join(`"cwd": `, escNew),
		innerOld:      innerOld,
		innerNew:      innerNew,
		deep:          deep,
	}
}

// line returns the rewritten line. The safe default only touches the
// structural "cwd" field; --deep also rewrites the path anywhere else it
// appears (tool arguments, message bodies).
func (p *pathRewriter) line(line []byte) []byte {
	line = bytes.ReplaceAll(line, p.cwdCompactOld, p.cwdCompactNew)
	line = bytes.ReplaceAll(line, p.cwdSpacedOld, p.cwdSpacedNew)
	if p.deep && len(p.innerOld) > 0 {
		line = bytes.ReplaceAll(line, p.innerOld, p.innerNew)
	}
	return line
}
