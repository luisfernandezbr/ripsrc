package incblame

import (
	"bufio"
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Blame contains blame information for a file, with commit hash that created each particular line.
type Blame struct {
	Commit string
	Lines  []Line
}

// Line contains actual data and commit hash for each line in the file.
type Line struct {
	Line   []byte
	Commit string
}

// String returns compact string representation of line. Useful in tests to see output.
func (l Line) String() string {
	return l.Commit + ":" + string(l.Line)
}

func (l Line) Eq(l2 Line) bool {
	if l.Commit != l2.Commit {
		return false
	}
	if !bytes.Equal(l.Line, l2.Line) {
		return false
	}
	return true
}

// String returns compact string representation of file. Useful in tests to see output.
func (f Blame) String() string {
	out := []string{f.Commit}
	if len(f.Lines) == 0 {
		out = append(out, "empty")
	}
	for i, l := range f.Lines {
		out = append(out, strconv.Itoa(i)+":"+l.String())
	}
	return strings.Join(out, "\n")
}

func (f Blame) Eq(f2 *Blame) bool {
	if f.Commit != f2.Commit {
		return false
	}
	if len(f.Lines) != len(f2.Lines) {
		return false
	}
	for i := range f.Lines {
		a := f.Lines[i]
		b := f2.Lines[i]
		if !a.Eq(b) {
			return false
		}
	}
	return true
}

// Apply create a new blame data for file based on diff and parent commit blame data.
func Apply(parents []Blame, diff Diff, commit string) Blame {
	if len(parents) == 0 {
		return applyOneParent(Blame{}, diff, commit)
	}
	if len(parents) == 1 {
		return applyOneParent(parents[0], diff, commit)
	}
	return applyMerge(parents, diff, commit)
}

// applyMerge creates a new file with blame data based on merge diff and parent blame data.
func applyMerge(parents []Blame, diff Diff, commit string) Blame {
	res := make([]Line, len(parents[0].Lines))
	copy(res, parents[0].Lines)

	remLine := func(i int) {
		if i > len(res) {
			panic(fmt.Errorf("trying to remove line which is not in blame, commit %v line %v blame %v", commit, i, parents[0]))
		}
		res = append(res[:i], res[i+1:]...)
	}
	addLine := func(i int, data []byte, commit string) {
		temp := []Line{}
		temp = append(temp, res[:i]...)
		temp = append(temp, Line{Line: data, Commit: commit})
		if i != len(res) {
			temp = append(temp, res[i:]...)
		}
		res = temp
	}

	sort.Slice(diff.Hunks, func(i, j int) bool {
		a := diff.Hunks[i]
		b := diff.Hunks[j]
		return a.Locations[0].Offset > b.Locations[0].Offset
	})

	for _, h := range diff.Hunks {
		scanner := bufio.NewScanner(bytes.NewReader(h.Data))
		scanner.Buffer(make([]byte, maxLine), maxLine)

		i := h.Locations[0].Offset - 1
		if i == -1 {
			i = 0
		}

		lenp := len(parents)
		// offsets are used to retrieve corresponding line owner from merge parent
		offsets := make([]int, lenp)
		for i := 0; i < lenp; i++ {
			offsets[i] = h.Locations[i].Offset - 2
		}

		for scanner.Scan() {
			b := scanner.Bytes()
			b = copyBytes(b)

			if len(b) == 0 {
				panic(fmt.Errorf("commit %v could not process patch line, it was empty, h.Data %v", commit, string(h.Data)))
			}

			if len(b) < len(parents) {
				panic(fmt.Errorf("commit %v could not process patch line, len(b) < len(parents) %v < %v, h.Data %v", commit, len(b), len(parents), string(h.Data)))
			}

			ops := b[0:lenp]
			data := b[lenp:]

			for i, v := range ops[1:] {
				i += 1
				switch v {
				case ' ', '\t':
					// if was removed from another parent, and was not in this one at all
					removedFromAnother := false
					for _, v := range ops {
						if v == '-' {
							removedFromAnother = true
						}
					}
					if removedFromAnother {
						// this line was not in parent, if it would be in parent it would be marked as - as well
					} else {
						offsets[i]++
					}
				case '-':
					offsets[i]--
				case '+':
				default:
					panic(fmt.Errorf("invalid patch line prefix, line '%s' prefix '%v' '%s'", b, v, string(v)))
				}
			}

			switch ops[0] {
			case ' ', '\t':
				// no change
				i++
			case '-':
				remLine(i)
				// no need to inc offset
			case '+':
				srcI := 0
				for i, op := range ops {
					if op == ' ' {
						srcI = i
					}
				}
				src := ""
				if srcI == 0 {
					// source is merge itself
					src = commit
				} else {
					offset := offsets[srcI]
					lines := parents[srcI].Lines
					if offset < 0 || offset >= len(lines) {
						panic(fmt.Errorf("invalid offset for merge parent, wanted %v, len(lines)=%v", offset, len(lines)))
					}
					src = lines[offset].Commit
				}
				addLine(i, data, src)
				i++
			default:
				panic(fmt.Errorf("invalid patch prefix, line %s prefix '%v' '%s'", b, ops[0], string(ops[0])))
			}
		}
		if err := scanner.Err(); err != nil {
			panic(err)
		}
	}

	return Blame{Lines: res, Commit: commit}
}

func applyOneParent(file Blame, diff Diff, commit string) Blame {
	res := make([]Line, len(file.Lines))
	copy(res, file.Lines)

	remLine := func(i int) {
		if i > len(res) {
			panic(fmt.Errorf("trying to remove line which is not in blame, commit %v line %v blame %v", commit, i, file))
		}
		res = append(res[:i], res[i+1:]...)
	}
	addLine := func(i int, data []byte) {
		temp := []Line{}
		temp = append(temp, res[:i]...)
		temp = append(temp, Line{Line: data, Commit: commit})
		if i != len(res) {
			temp = append(temp, res[i:]...)
		}
		res = temp
	}

	sort.Slice(diff.Hunks, func(i, j int) bool {
		a := diff.Hunks[i]
		b := diff.Hunks[j]
		return a.Locations[0].Offset > b.Locations[0].Offset
	})

	for _, h := range diff.Hunks {
		if len(h.Locations) == 0 {
			panic(fmt.Errorf("no location in diff hunk %+v", h))
		}

		scanner := bufio.NewScanner(bytes.NewReader(h.Data))
		scanner.Buffer(nil, maxLine)

		i := h.Locations[0].Offset - 1
		if i == -1 {
			i = 0
		}

		for scanner.Scan() {
			b := scanner.Bytes()
			if len(b) == 0 {
				panic(fmt.Errorf("could not process patch line, it was empty, h.Data %v", string(h.Data)))
			}
			b = copyBytes(b)

			op := b[0]
			data := b[1:]
			switch op {
			case ' ', '\t':
				// no change
				i++
			case '-':
				remLine(i)
				// no need to inc offset
			case '+':
				addLine(i, data)
				i++
			case 92:
				if string(b) == "\\ No newline at end of file" {
					// can ignore this, we do not case about end of file newline
					continue
				}
				panic(fmt.Errorf("invalid patch line, starts with \\ but not 'No newline at end of file', line '%s'", b))

			default:
				panic(fmt.Errorf("invalid patch prefix, line %s prefix %v commit %v", b, op, commit))
			}
		}
		if err := scanner.Err(); err != nil {
			panic(err)
		}
	}

	return Blame{Lines: res, Commit: commit}
}

func copyBytes(b []byte) []byte {
	res := make([]byte, len(b))
	copy(res, b)
	return res
}

func copyLines(lines []Line) (res []Line) {
	res = make([]Line, len(lines))
	copy(res, lines)
	return
}
