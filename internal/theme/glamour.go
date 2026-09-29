package theme

import (
	"strings"

	"charm.land/glamour/v2/ansi"
)

// GlamourStyle derives a Glamour style from the palette's role tokens and the
// glyph set. Glamour v2 has no auto light/dark, so the palette's variant and
// depth decide everything; Bubble Tea downsamples the output. Code blocks are
// not syntax-highlighted: Glamour registers its Chroma style once per process
// and its formatter always emits 256-colour codes, which would ignore the
// theme and the colour depth.
func GlamourStyle(p Palette, g Glyphs) ansi.StyleConfig {
	prim := p.primitive
	block := func(r Role) ansi.StyleBlock { return ansi.StyleBlock{StylePrimitive: prim(r)} }
	heading := func(prefix string, r Role) ansi.StyleBlock {
		b := block(r)
		b.Prefix = prefix
		b.Bold = boolPtr(true)
		return b
	}

	text := prim(Text)
	code := block(Primary)
	code.Prefix, code.Suffix = " ", " "

	quote := block(Dim)
	quote.Indent = uintPtr(1)
	quote.IndentToken = stringPtr(g.Vertical + " ")

	link := prim(Primary)
	link.Underline = boolPtr(true)

	task := ansi.StyleTask{Ticked: g.Checked + " ", Unticked: g.Unchecked + " "}
	task.StylePrimitive = prim(Text)

	doc := block(Text)
	doc.Margin = uintPtr(0)
	doc.BlockPrefix, doc.BlockSuffix = "", ""

	emph := prim(Text)
	emph.Italic = boolPtr(true)
	strong := prim(Strong)
	strong.Bold = boolPtr(true)
	strike := prim(Dim)
	strike.CrossedOut = boolPtr(true)

	hr := prim(Rule)
	hr.Format = "\n" + strings.Repeat(g.Rule, 8) + "\n"

	item := prim(Text)
	item.BlockPrefix = g.Bullet + " "
	enumeration := prim(Text)
	enumeration.BlockPrefix = ". "

	codeBlock := ansi.StyleCodeBlock{StyleBlock: block(Dim)}
	codeBlock.Margin = uintPtr(0)

	table := ansi.StyleTable{StyleBlock: block(Text)}
	table.CenterSeparator = stringPtr("┼")
	table.ColumnSeparator = stringPtr(g.Vertical)
	table.RowSeparator = stringPtr(g.Rule)
	if g.Tier == TierASCII {
		table.CenterSeparator = stringPtr("+")
	}

	return ansi.StyleConfig{
		Document:       doc,
		BlockQuote:     quote,
		Paragraph:      block(Text),
		List:           ansi.StyleList{StyleBlock: block(Text), LevelIndent: 2},
		Heading:        ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{BlockSuffix: "\n", Color: prim(Strong).Color, Bold: boolPtr(true)}},
		H1:             heading("# ", Primary),
		H2:             heading("## ", Primary),
		H3:             heading("### ", Strong),
		H4:             heading("#### ", Strong),
		H5:             heading("##### ", Strong),
		H6:             heading("###### ", Dim),
		Text:           text,
		Strikethrough:  strike,
		Emph:           emph,
		Strong:         strong,
		HorizontalRule: hr,
		Item:           item,
		Enumeration:    enumeration,
		Task:           task,
		Link:           link,
		LinkText:       prim(Primary),
		Image:          link,
		ImageText:      prim(Dim),
		Code:           code,
		CodeBlock:      codeBlock,
		Table:          table,
		DefinitionList: block(Text),
		DefinitionTerm: strong,
		HTMLBlock:      block(Dim),
		HTMLSpan:       block(Dim),
	}
}

func (p Palette) primitive(r Role) ansi.StylePrimitive {
	fg, _, a := p.parts(r)
	var s ansi.StylePrimitive
	if fg != "" {
		s.Color = &fg
	}
	if a.Bold {
		s.Bold = boolPtr(true)
	}
	if a.Faint {
		s.Faint = boolPtr(true)
	}
	if a.Underline {
		s.Underline = boolPtr(true)
	}
	return s
}

func boolPtr(b bool) *bool       { return &b }
func stringPtr(s string) *string { return &s }
func uintPtr(u uint) *uint       { return &u }
