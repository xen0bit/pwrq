package prompt

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// StateBlock is the state as the block before "QUESTION:": text as it is,
// anything else as indented JSON, then a blank line. ok is false for no
// state (nil), where questions follow the media directly.
func StateBlock(state any) (string, error) {
	if s, ok := state.(string); ok {
		return s + "\n\n", nil
	}
	js, err := PythonJSON(state, 2)
	if err != nil {
		return "", err
	}
	return js + "\n\n", nil
}

// StateText is the state as the prompt renders it, without the blank line
// after it: what a read's span offsets index.
func StateText(state any) (string, error) {
	block, err := StateBlock(state)
	if err != nil {
		return "", err
	}
	return block[:len(block)-2], nil
}

// Plain is text as the caller's text: no special token in it once encoded.
func (b *Builder) Plain(text string) string {
	out, _ := b.plain(text, false)
	return out
}

// plain inserts a zero-width space after the first character of each special
// token in text; with offsets it also maps every character boundary of the
// escaped text to the original's (in characters).
func (b *Builder) plain(text string, offsets bool) (string, []int) {
	var boundaries []int
	if offsets {
		n := len([]rune(text))
		boundaries = make([]int, n+1)
		for i := range boundaries {
			boundaries[i] = i
		}
	}
	if b.specials == nil {
		return text, boundaries
	}
	for { // again for a match that overlapped the one broken before it
		locs := b.specials.FindAllStringIndex(text, -1)
		if len(locs) == 0 {
			return text, boundaries
		}
		if offsets {
			// Insert boundaries right to left, each at its match's second character.
			for i := len(locs) - 1; i >= 0; i-- {
				at := len([]rune(text[:locs[i][0]])) + 1
				boundaries = slices.Insert(boundaries, at, boundaries[at])
			}
		}
		var sb strings.Builder
		last := 0
		for _, l := range locs {
			sb.WriteString(text[last:l[0]])
			m := text[l[0]:l[1]]
			r := []rune(m)
			sb.WriteString(string(r[0]) + breakChar + string(r[1:]))
			last = l[1]
		}
		sb.WriteString(text[last:])
		text = sb.String()
	}
}

func (b *Builder) edgeLegend() (string, error) {
	g, scale := b.edgeGrid()
	if scale > 1 {
		return "Each edge is a number from 0 to 999, in thousandths of the picture: top and bottom from its top edge, " +
			"left and right from its left edge.", nil
	}
	labels := make([]string, g.Size)
	for i := range labels {
		labels[i] = strconv.Itoa(i)
	}
	as, err := b.optionAliases(labels)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Each edge is a code on a %d-value scale: %s is 0 (top or left), %s is 1 "+
		"(bottom or right). Values are equally spaced.", g.Size, as[0].code, as[len(as)-1].code), nil
}

func optionLine(code string, o Option) string {
	text := strings.ReplaceAll(o.Name, "_", " ")
	if o.Desc != nil && *o.Desc != "" {
		text = *o.Desc
	}
	return code + " " + text
}

func pyStr(s *string) string {
	if s == nil {
		return "None"
	}
	return *s
}

// questionBlock is the question as the user turn writes it.
func (b *Builder) questionBlock(q Question) (string, error) {
	said := q.Instructions
	switch {
	case q.Type == Box:
		slots := b.Slots(q)
		one := "one code"
		if slots[len(slots)-1].kind == slotNumber {
			one = "one number"
		}
		legend, err := b.edgeLegend()
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s\n\nPresence is yes or no. %s Reply with yes or no at present, then %s at top, bottom, left and right.", said, legend, one), nil
	case q.Type == Boxes:
		most := ""
		if b.cfg.Version == 1 {
			most = fmt.Sprintf("%d at most, ", Instances)
		}
		legend, err := b.edgeLegend()
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s\n\nEvery one of them, %sin reading order (left edge, then top). Present k is yes when there are at least k. %s Reply with yes or no at each present, then its top, bottom, left and right.", said, most, legend), nil
	case q.Estimate != nil:
		g := *q.Estimate
		unit := ""
		if q.Unit != nil {
			unit = " " + *q.Unit
		}
		if b.Slots(q)[0].kind == slotNumber {
			return fmt.Sprintf("%s\n\nEstimate from %s to %s%s, in steps of %s. Reply with the number only.",
				said, pyG(g.Low), pyG(g.High), unit, pyG(g.Step())), nil
		}
		bins := gridQuestion(g)
		var codes []string
		if bins.kind == slotScore {
			for i := 0; i < g.Size; i++ {
				codes = append(codes, strconv.Itoa(i))
			}
		} else {
			as, err := b.optionAliases(bins.labels)
			if err != nil {
				return "", err
			}
			for _, a := range as {
				codes = append(codes, a.code)
			}
		}
		return fmt.Sprintf("%s\n\nEstimate from %s to %s%s on %d equally spaced values, step %s: %s is %s, %s is %s. Reply with the code only.",
			said, pyG(g.Low), pyG(g.High), unit, g.Size, pyG(g.Step()), codes[0], pyG(g.Low), codes[len(codes)-1], pyG(g.High)), nil
	case q.IsRead():
		unit := ""
		if q.Type == Number && q.Unit != nil && *q.Unit != "" {
			unit = fmt.Sprintf(" (unit: %s)", *q.Unit)
		}
		return fmt.Sprintf("%s\n\nRead the %s%s from the state, or nothing if it is absent.", said, q.Type, unit), nil
	case q.Type == Choice:
		if q.Multiple {
			lines := make([]string, len(q.Options))
			for i, o := range q.Options {
				name, _ := PythonJSON(o.Name, -1)
				text := o.Name
				if o.Desc != nil && *o.Desc != "" {
					text = *o.Desc
				}
				lines[i] = name + ": " + text
			}
			return fmt.Sprintf("%s\n\nOptions:\n%s\n\nReply with yes or no for each option.", said, strings.Join(lines, "\n")), nil
		}
		labels := make([]string, len(q.Options))
		for i, o := range q.Options {
			labels[i] = o.Name
		}
		as, err := b.optionAliases(labels)
		if err != nil {
			return "", err
		}
		lines := make([]string, len(q.Options))
		for i, o := range q.Options {
			lines[i] = optionLine(as[i].code, o)
		}
		return fmt.Sprintf("%s\n\nOptions:\n%s\n\nReply with the option code only.", said, strings.Join(lines, "\n")), nil
	case q.Type == Noul:
		extra := ""
		if q.HasNoul {
			extra = fmt.Sprintf("\nYes: %s\nNo: %s", pyStr(q.NoulYes), pyStr(q.NoulNo))
		}
		return fmt.Sprintf("%s%s\n\nReply with yes or no only.", said, extra), nil
	case q.Type == Score:
		lines := make([]string, len(q.Levels))
		for i, l := range q.Levels {
			lines[i] = fmt.Sprintf("%d %s", i, l)
		}
		return fmt.Sprintf("%s\n\n%s\n\nReply with a single digit 0-%d only.", said, strings.Join(lines, "\n"), len(q.Levels)-1), nil
	}
	return "", fmt.Errorf("unknown question type %q", q.Type)
}

// suffixLayout is the question and the assistant header up to its answer
// slots, and each slot's token offset from the suffix's last token.
func (b *Builder) suffixLayout(q Question) (string, []int, error) {
	body, err := b.questionBlock(q)
	if err != nil {
		return "", nil, err
	}
	switch {
	case q.IsRead():
		return b.markedSuffix(body, []string{"start", "end"}, "")
	case q.Type == Box:
		return b.markedSuffix(body, append([]string{"present"}, edges...), "")
	case q.Type == Boxes:
		var names []string
		for k := 1; k <= q.blocks(); k++ {
			for _, n := range append([]string{"present"}, edges...) {
				names = append(names, fmt.Sprintf("%s %d", n, k))
			}
		}
		// The last block ends on the newline a next block would follow, where
		// the colon and it are one token.
		after := ""
		if q.blocks() < Instances || b.cfg.Version > 1 {
			after = "\n"
		}
		return b.markedSuffix(body, names, after)
	case q.Type == Choice && q.Multiple:
		names := make([]string, len(q.Options))
		for i, o := range q.Options {
			names[i], _ = PythonJSON(o.Name, -1)
		}
		return b.markedSuffix(body, names, "")
	}
	return b.Plain(body) + imEnd + "\n" + imStart + "assistant\n" + b.cfg.Lead, []int{0}, nil
}

func isBlank(s string) bool { return strings.TrimFunc(s, unicode.IsSpace) == "" }

// markedSuffix writes a question's named answer markers and finds each
// one's colon token, as an offset from the suffix's last token.
func (b *Builder) markedSuffix(body string, names []string, after string) (string, []int, error) {
	seen := map[string]bool{}
	for _, n := range names {
		if n == "" || strings.Contains(n, "\n") || seen[n] {
			return "", nil, fmt.Errorf("slot names must be distinct nonempty single lines")
		}
		seen[n] = true
	}
	text := b.Plain(body) + imEnd + "\n" + imStart + "assistant\n" + b.cfg.Lead
	var ends []int // in characters
	for i, n := range names {
		if i > 0 {
			text += "\n"
		}
		text += b.Plain(n) + ":"
		ends = append(ends, len([]rune(text)))
	}
	if after != "" {
		ids := b.tok.Encode(text + after)
		if strings.HasSuffix(strings.TrimRightFunc(b.tok.Decode(ids[len(ids)-1:]), unicode.IsSpace), ":") {
			text += after
		}
	}
	ids, offs := b.tok.EncodeOffsets(text)
	runes := []rune(text)
	var slots []int
	for _, chars := range ends {
		end, stop, count := -1, 0, 0
		for i, o := range offs {
			if o.Start <= chars-1 && chars-1 < o.End {
				end, stop = i, o.End
				count++
			}
		}
		if count != 1 {
			return "", nil, fmt.Errorf("a slot marker must have an exact colon token position")
		}
		// The colon may share its token with following whitespace, never with the next marker's text.
		if !isBlank(string(runes[chars:stop])) || !strings.HasSuffix(strings.TrimRightFunc(b.tok.Decode(ids[end:end+1]), unicode.IsSpace), ":") {
			return "", nil, fmt.Errorf("a slot marker token must end with its colon and optional whitespace")
		}
		slots = append(slots, end-len(ids)+1)
	}
	return text, slots, nil
}

// prefixText is everything before the question, shared by all questions on
// one state: the media's markup at the head of the user turn, then the
// state. With no state the question follows the media directly.
func (b *Builder) prefixText(state any, media string) (string, error) {
	turn := ""
	if b.cfg.System != "" {
		turn = imStart + "system\n" + b.cfg.System + imEnd + "\n"
	}
	body := ""
	if state != nil {
		block, err := StateBlock(state)
		if err != nil {
			return "", err
		}
		body = b.Plain(block) + "\n" + header
	}
	return b.cfg.BOS + turn + imStart + "user\n" + media + body, nil
}

func questionHeader(state any) string {
	if state == nil {
		return ""
	}
	return header
}

// Row is one packed sequence: a state and up to SetSize of its questions.
type Row struct {
	IDs []int32
	// QuestionIDs is -1 on the state and k on the k-th question of the row.
	QuestionIDs []int64
	// Slots are the row's answer positions in order, each with the question
	// (an index into the request's questions) and the slot it reads.
	Slots []RowSlot
	// Positions maps the row's tokens to the state text, for reads.
	Positions *StatePositions
	// Text is the prompt as text, for debugging and the fixtures.
	Text string
}

// RowSlot is one answer position of a row.
type RowSlot struct {
	Question int
	Slot     Slot
	Pos      int
}

// SlotPositions are the row's answer positions, as the decoder takes them.
func (r Row) SlotPositions() []int64 {
	out := make([]int64, len(r.Slots))
	for i, s := range r.Slots {
		out[i] = int64(s.Pos)
	}
	return out
}

// Rows builds a request's rows: its media's markup (from vision) at the
// head of every row, then the state, then up to setSize questions packed
// after it, each ending at its answer slots.
func (b *Builder) Rows(state any, questions []Named, media string, setSize int) ([]Row, error) {
	if setSize <= 0 {
		setSize = b.cfg.SetSize
	}
	prefix, err := b.prefixText(state, media)
	if err != nil {
		return nil, err
	}
	suffixes := make([]string, len(questions))
	offsets := make([][]int, len(questions))
	for i, nq := range questions {
		suffixes[i], offsets[i], err = b.suffixLayout(nq.Question)
		if err != nil {
			return nil, fmt.Errorf("question %q: %w", nq.Name, err)
		}
	}
	var rows []Row
	for k := 0; k < len(questions); k += setSize {
		end := min(k+setSize, len(questions))
		text := prefix + suffixes[k]
		reads := false
		for _, nq := range questions[k:end] {
			reads = reads || nq.Question.IsRead()
		}
		var ids []int32
		var offs []Offset
		if reads {
			var to []tokOffset
			ids, to = b.encodeOffsets(text)
			offs = to
		} else {
			ids = b.tok.Encode(text)
		}
		// pack_set: the first question tokenized as its lone prompt is, each
		// further one its own group, the header then its suffix.
		stateLen := len(ids) - len(b.tok.Encode(suffixes[k]))
		qids := make([]int64, len(ids))
		for i := range qids {
			if i < stateLen {
				qids[i] = -1
			} else {
				qids[i] = 1
			}
		}
		row := Row{Text: text}
		ends := [][]int{positions(offsets[k], len(ids)-1)}
		hdr := questionHeader(state)
		for j := k + 1; j < end; j++ {
			own := b.tok.Encode(hdr + suffixes[j])
			ids = append(ids, own...)
			for range own {
				qids = append(qids, int64(j-k+1))
			}
			ends = append(ends, positions(offsets[j], len(ids)-1))
			row.Text += hdr + suffixes[j]
		}
		row.IDs, row.QuestionIDs = ids, qids
		for j := k; j < end; j++ {
			slots := b.Slots(questions[j].Question)
			if len(slots) != len(ends[j-k]) {
				return nil, fmt.Errorf("question %q: %d slots and %d answer positions", questions[j].Name, len(slots), len(ends[j-k]))
			}
			for s, slot := range slots {
				row.Slots = append(row.Slots, RowSlot{Question: j, Slot: slot, Pos: ends[j-k][s]})
			}
		}
		if reads {
			sp, err := b.statePositions(state, prefix, text, offs)
			if err != nil {
				return nil, err
			}
			row.Positions = sp
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// tokOffset is a token's character span.
type tokOffset = Offset

// Offset is a token's span in characters.
type Offset struct{ Start, End int }

func (b *Builder) encodeOffsets(text string) ([]int32, []Offset) {
	ids, offs := b.tok.EncodeOffsets(text)
	out := make([]Offset, len(offs))
	for i, o := range offs {
		out[i] = Offset{o.Start, o.End}
	}
	return ids, out
}

func positions(own []int, end int) []int {
	out := make([]int, len(own))
	for i, o := range own {
		out[i] = end + o
	}
	return out
}
