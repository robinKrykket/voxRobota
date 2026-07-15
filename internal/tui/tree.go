package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type treeNode struct {
	path     string
	name     string
	isDir    bool
	expanded bool
	loaded   bool
	depth    int
	children []*treeNode
}

// Tree is a lazily-loaded, keyboard/mouse navigable directory tree.
type Tree struct {
	root   *treeNode
	flat   []*treeNode
	cursor int
	offset int
	width  int
	height int
}

func NewTree(rootPath string) Tree {
	abs, err := filepath.Abs(rootPath)
	if err != nil {
		abs = rootPath
	}
	root := &treeNode{path: abs, name: filepath.Base(abs), isDir: true, expanded: true}
	t := Tree{root: root}
	t.load(root)
	t.reflatten()
	return t
}

func (t *Tree) load(n *treeNode) {
	if n.loaded || !n.isDir {
		return
	}
	n.loaded = true
	entries, err := os.ReadDir(n.path)
	if err != nil {
		return
	}
	kids := make([]*treeNode, 0, len(entries))
	for _, e := range entries {
		kids = append(kids, &treeNode{
			path:  filepath.Join(n.path, e.Name()),
			name:  e.Name(),
			isDir: e.IsDir(),
			depth: n.depth + 1,
		})
	}
	sort.SliceStable(kids, func(i, j int) bool {
		if kids[i].isDir != kids[j].isDir {
			return kids[i].isDir // directories first
		}
		return strings.ToLower(kids[i].name) < strings.ToLower(kids[j].name)
	})
	n.children = kids
}

func (t *Tree) reflatten() {
	t.flat = t.flat[:0]
	var walk func(n *treeNode)
	walk = func(n *treeNode) {
		t.flat = append(t.flat, n)
		if n.isDir && n.expanded {
			for _, c := range n.children {
				walk(c)
			}
		}
	}
	walk(t.root)
	if t.cursor >= len(t.flat) {
		t.cursor = len(t.flat) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
}

func (t *Tree) SetSize(w, h int) {
	t.width = w
	t.height = h
	t.ensureVisible()
}

func (t *Tree) Move(d int) {
	t.cursor += d
	if t.cursor < 0 {
		t.cursor = 0
	}
	if t.cursor >= len(t.flat) {
		t.cursor = len(t.flat) - 1
	}
	t.ensureVisible()
}

func (t *Tree) ensureVisible() {
	if t.height <= 0 {
		return
	}
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+t.height {
		t.offset = t.cursor - t.height + 1
	}
	if t.offset < 0 {
		t.offset = 0
	}
}

func (t *Tree) Current() *treeNode {
	if t.cursor < 0 || t.cursor >= len(t.flat) {
		return nil
	}
	return t.flat[t.cursor]
}

// Enter expands/collapses a directory, or returns (path, true) when a file
// should be opened.
func (t *Tree) Enter() (string, bool) {
	n := t.Current()
	if n == nil {
		return "", false
	}
	if n.isDir {
		if n.expanded {
			n.expanded = false
		} else {
			t.load(n)
			n.expanded = true
		}
		t.reflatten()
		t.ensureVisible()
		return "", false
	}
	return n.path, true
}

func (t *Tree) Collapse() {
	n := t.Current()
	if n == nil {
		return
	}
	if n.isDir && n.expanded {
		n.expanded = false
		t.reflatten()
		t.ensureVisible()
		return
	}
	parent := filepath.Dir(n.path)
	for i, fn := range t.flat {
		if fn.path == parent {
			t.cursor = i
			t.ensureVisible()
			return
		}
	}
}

// ClickRow selects the node at a viewport row (0-based from the top).
func (t *Tree) ClickRow(row int) {
	idx := t.offset + row
	if idx >= 0 && idx < len(t.flat) {
		t.cursor = idx
		t.ensureVisible()
	}
}

func (t *Tree) View() string {
	end := t.offset + t.height
	if end > len(t.flat) {
		end = len(t.flat)
	}
	lines := make([]string, 0, t.height)
	for i := t.offset; i < end; i++ {
		n := t.flat[i]
		indent := strings.Repeat("  ", n.depth)
		icon := "  "
		if n.isDir {
			if n.expanded {
				icon = "▾ "
			} else {
				icon = "▸ "
			}
		}
		name := n.name
		if n.isDir {
			name += "/"
		}
		row := " " + truncate(indent+icon+name, t.width-1)
		if i == t.cursor {
			lines = append(lines, selectedRow.Render(padRight(row, t.width)))
		} else {
			style := textStyle
			switch {
			case strings.HasPrefix(n.name, "."):
				style = dimStyle
			case n.isDir:
				style = subtitleStyle
			}
			lines = append(lines, style.Render(row))
		}
	}
	for len(lines) < t.height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
