// Package parsing turns the text d1 points at into values: a number with
// its currency or percent set apart, or an ISO date. It is a port of d1's
// parsing.py, and keeps its rule: strict readings of what is written, with no
// arithmetic, no unit conversion and no invented date component. A reading
// the text does not settle is no value, never a guess.
//
// The patterns use look-behind and back-references, which Go's regexp does
// not have, so they run on regexp2 with Python's semantics for \d, \w and \b
// (Unicode in both).
package parsing

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/dlclark/regexp2"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// Month names a date is written with (accents folded; Polish and Russian in
// the genitive a date uses), and weekdays, which a date may lead with.
var monthNames = []string{
	"january janvier januar enero gennaio janeiro januari stycznia styczen января",
	"february fevrier februar febrero febbraio fevereiro februari lutego luty февраля",
	"march mars marz marzo marco maart marca марта",
	"april avril abril aprile kwietnia kwiecien апреля",
	"may mai mayo maggio maio mei maj maja мая",
	"june juin juni junio giugno junho czerwca czerwiec июня",
	"july juillet juli julio luglio julho lipca lipiec июля",
	"august aout agosto augustus augusti sierpnia sierpien августа",
	"september septembre septiembre settembre setembro wrzesnia wrzesien сентября",
	"october octobre oktober octubre ottobre outubro pazdziernika pazdziernik октября",
	"november novembre noviembre novembro listopada listopad ноября",
	"december decembre dezember diciembre dicembre dezembro grudnia grudzien декабря",
}

var (
	months   = map[string]int{}
	weekdays = map[string]bool{}
)

const weekdayNames = `monday tuesday wednesday thursday friday saturday sunday mon tue tues wed thu thur thurs fri sat sun
lundi mardi mercredi jeudi vendredi samedi dimanche montag dienstag mittwoch donnerstag freitag samstag sonntag lunes
martes miercoles jueves viernes sabado domingo lunedi martedi mercoledi giovedi venerdi sabato domenica segunda terca
quarta quinta sexta`

// Currencies by the decimal mark their amounts are written with: "" for
// none (whole units), "both" for both in use.
var currencies = map[string][]string{
	".":    {"$", "£", "₹", "USD", "GBP", "CAD", "AUD", "NZD", "INR", "Rs.", "Rs", "MXN", "CNY", "RMB", "元", "HKD", "SGD"},
	",":    {"R$", "BRL", "₽", "RUB", "zł", "PLN", "kr", "SEK", "NOK", "DKK", "₺", "TRY"},
	"":     {"¥", "￥", "円", "₩", "₫", "Rp.", "Rp", "IDR", "JPY", "KRW", "VND", "CLP"},
	"both": {"€", "EUR", "CHF"},
}

var (
	names = map[string]string{} // casefolded -> as written
	marks = map[string]string{} // casefolded -> decimal mark
)

// Scale words, written apart or attached: unambiguous, so no context needed
// (German "Billion" is 10^12: left out).
var scales = map[string]int64{}

var (
	reCurrency, reCJKDate, reNumericDate, reWordDate                       *regexp2.Regexp
	reScale, reSuffix, reCounted, reGrouped, reShort, reRepeated, reLocale *regexp2.Regexp
	reDateLocale, reTimed, reOrdinal, reDegree, reArticle, reDigitsOnly    *regexp2.Regexp
	reSpacedGroup, reLakh, reRupee, reMarkup                               *regexp2.Regexp
	dateFormats                                                            []dateFormat
	reCurrencyFull, reCurrencySplit                                        *regexp2.Regexp
)

type dateFormat struct {
	re    *regexp2.Regexp
	order string
}

func mustRE(pattern string) *regexp2.Regexp {
	re := regexp2.MustCompile(pattern, regexp2.None)
	re.MatchTimeout = time.Second
	return re
}

func escape(s string) string { return regexp2.Escape(s) }

func init() {
	for i, line := range monthNames {
		for _, n := range strings.Fields(line) {
			months[n] = i + 1
		}
	}
	abbreviations := map[string]map[int]bool{}
	for n, m := range months {
		k := string([]rune(n)[:min(3, len([]rune(n)))])
		if abbreviations[k] == nil {
			abbreviations[k] = map[int]bool{}
		}
		abbreviations[k][m] = true
	}
	for k, ms := range abbreviations {
		if len(ms) == 1 {
			for m := range ms {
				months[k] = m
			}
		}
	}
	months["janv"], months["fevr"], months["juil"], months["sept"] = 1, 2, 7, 9
	for _, w := range strings.Fields(weekdayNames) {
		weekdays[w] = true
	}
	var all []string
	for mark, cs := range currencies {
		for _, c := range cs {
			f := fold(c)
			names[f] = c
			marks[f] = mark
			all = append(all, c)
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if len([]rune(all[i])) != len([]rune(all[j])) {
			return len([]rune(all[i])) > len([]rune(all[j]))
		}
		return all[i] < all[j]
	})
	var alts []string
	for _, c := range all {
		r := []rune(c)[0]
		if r < 128 && unicode.IsLetter(r) {
			alts = append(alts, `\b`+escape(c)+`(?![a-z])`)
		} else {
			alts = append(alts, escape(c))
		}
	}
	currency := "(?i:" + strings.Join(alts, "|") + ")"
	reCurrency = mustRE(currency)
	reCurrencyFull = mustRE(`\A` + currency + `\s*\z`)
	reCurrencySplit = mustRE(`\A(?:(` + currency + `)\s*(.*)|(.*?)\s*(` + currency + `))\z`)

	for _, group := range []struct {
		words []string
		scale int64
	}{
		{[]string{"thousand", "mil", "tausend", "тыс", "千"}, 1e3},
		{[]string{"万", "萬"}, 1e4},
		{[]string{"lakh", "lakhs"}, 1e5},
		{[]string{"crore", "crores"}, 1e7},
		{[]string{"million", "millions", "mio", "mln", "millón", "millones", "milhão", "milhões", "milione", "milioni", "млн"}, 1e6},
		{[]string{"亿", "億"}, 1e8},
		{[]string{"billion", "billions", "milliard", "milliards", "milliarde", "milliarden", "mrd", "miliardo", "miliardi", "млрд"}, 1e9},
		{[]string{"trillion"}, 1e12},
	} {
		for _, w := range group.words {
			scales[w] = group.scale
		}
	}
	var scaleWords []string
	for w := range scales {
		scaleWords = append(scaleWords, escape(w))
	}
	sort.SliceStable(scaleWords, func(i, j int) bool {
		if len(scaleWords[i]) != len(scaleWords[j]) {
			return len(scaleWords[i]) > len(scaleWords[j])
		}
		return scaleWords[i] < scaleWords[j]
	})
	reScale = mustRE(`(?i)\s*(` + strings.Join(scaleWords, "|") + `)\.?\z`)
	reSuffix = mustRE(`(?i)(?<=\d)(k|m|bn|b)\z`)
	reCounted = mustRE(`(?i)\b(?:money|price|invoice|revenue|cost|count|quantity|population|people|items|units|users|downloads)\b`)
	reGrouped = mustRE(`(?<![\d.,])\d{1,3}(?:([,. \x{00a0}\x{202f}'’])\d{3})+(?!\1)([.,])\d+(?![\d.,])`)
	reShort = mustRE(`(?<![\d.,])\d+([.,])\d{1,2}(?![\d.,])`)
	reRepeated = mustRE(`(?<![\d.,])\d{1,3}(?:([.,])\d{3}){2,}(?![\d.,])`)
	reLocale = mustRE(`\b(?:locale|language|lang)["']?\s*[=:]\s*["']?(en|fr|de|es|it|pt|nl|pl|ru|sv|ja|zh|ko)(?:[-_][a-z]{2})?\b`)
	reCJKDate = mustRE(`(\d{4})\s*[年년]\s*(\d{1,2})\s*[月월]\s*(\d{1,2})\s*[日일]`)
	reNumericDate = mustRE(`(?<!\d)(\d{1,4})([/.-])(\d{1,2})\2(\d{2,4})(?!\d)`)
	var monthKeys []string
	for n := range months {
		monthKeys = append(monthKeys, n)
	}
	sort.SliceStable(monthKeys, func(i, j int) bool {
		if len([]rune(monthKeys[i])) != len([]rune(monthKeys[j])) {
			return len([]rune(monthKeys[i])) > len([]rune(monthKeys[j]))
		}
		return monthKeys[i] < monthKeys[j]
	})
	monthAlt := strings.Join(monthKeys, "|")
	reWordDate = mustRE(`\b(?:\d{1,2}\w*[ .]+(?:de\s+)?(?:` + monthAlt + `)[ ,.]+(?:de\s+)?\d{4}|(?:` + monthAlt + `)[ .]+\d{1,2}\w*[ ,.]+\d{4})\b`)
	day := `(?:dd?|jj?|tt?|gg?|day|jour|tag|dia|giorno)`
	month := `(?:mm?|month|mois|monat|mes|mese)`
	year := `(?:yy(?:yy)?|aa(?:aa)?|jj(?:jj)?|year|annee|jahr|ano|anno)`
	dateFormats = []dateFormat{
		{mustRE(`\b` + day + `\s*[/.-]\s*` + month + `\s*[/.-]\s*` + year + `\b`), "dmy"},
		{mustRE(`\b` + month + `\s*[/.-]\s*` + day + `\s*[/.-]\s*` + year + `\b`), "mdy"},
	}
	reDateLocale = mustRE(`\b(?:locale|language|lang|langue|sprache|idioma|lingua)["']?\s*[=:]\s*["']?(en(?:[-_](?:us|gb|au|ca))?|fr(?:[-_]fr)?|de(?:[-_]de)?|es(?:[-_]es)?|it(?:[-_]it)?)\b`)
	reTimed = mustRE(`\A(.+?)[T\s]+(\d{1,2}:\d{2}(?::\d{2}(?:\.\d+)?)?(?:Z|[+-]\d{2}:\d{2})?)\z`)
	reOrdinal = mustRE(`(?<=\d)(?:st|nd|rd|th|er)\b`)
	reDegree = mustRE(`(?<=\d)\.?(?:o|a|°)\b`)
	reArticle = mustRE(`\b(?:de|of|the|le|el|il|den)\b`)
	reDigitsOnly = mustRE(`\A\d[\d., '’]*\z`)
	reSpacedGroup = mustRE(`\A(\d{1,3}(?:[ '’]\d{3})+)([.,]\d+)?\z`)
	reLakh = mustRE(`\A\d{1,2}(?:,\d{2})+,\d{3}(?:\.\d+)?\z`)
	reRupee = mustRE(`(?i)₹|\bINR\b|\bRs\b|lakh|crore`)
	reMarkup = mustRE(`<\?[^>]*\?>`)
}

var folder = cases.Fold()

// fold is Python's casefold.
func fold(s string) string { return folder.String(s) }

// foldAccents is parsing.py's _fold: casefolded, decomposed, combining marks
// dropped.
func foldAccents(s string) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(fold(s)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// matches returns every match's groups (index 0 the whole match; "" for a
// group that did not take part).
func matches(re *regexp2.Regexp, s string) [][]string {
	var out [][]string
	m, _ := re.FindStringMatch(s)
	for m != nil {
		groups := m.Groups()
		row := make([]string, len(groups))
		for i, g := range groups {
			if len(g.Captures) > 0 {
				row[i] = g.String()
			}
		}
		out = append(out, row)
		m, _ = re.FindNextMatch(m)
	}
	return out
}

func first(re *regexp2.Regexp, s string) []string {
	if all := matches(re, s); len(all) > 0 {
		return all[0]
	}
	return nil
}

func isMatch(re *regexp2.Regexp, s string) bool {
	ok, _ := re.MatchString(s)
	return ok
}

// fullMatch anchors a pattern the way Python's re.fullmatch does.
func fullMatch(pattern *regexp2.Regexp, s string) []string {
	m := first(pattern, s)
	if m == nil || m[0] != s {
		return nil
	}
	return m
}

func one(values map[string]bool) (string, bool) {
	if len(values) != 1 {
		return "", false
	}
	for v := range values {
		return v, true
	}
	return "", false
}

// WrittenUnit returns only an explicitly written percent or currency marker;
// a currency sign stays a sign.
func WrittenUnit(text string) (string, bool) {
	if strings.HasSuffix(strings.TrimRightFunc(text, unicode.IsSpace), "%") {
		return "%", true
	}
	found := matches(reCurrency, text)
	if len(found) == 1 {
		return names[fold(found[0][0])], true
	}
	return "", false
}

func currencyMarks(text string) map[string]bool {
	out := map[string]bool{}
	for _, m := range matches(reCurrency, text) {
		if mark := marks[fold(m[0])]; mark != "both" {
			out[mark] = true
		}
	}
	return out
}

// decimalMark is the decimal mark ("" when amounts have none) from the first
// evidence that speaks: the span's own currency, the document's other
// numbers, the document's currencies, a declared locale.
func decimalMark(written, context string) (string, bool) {
	context, _ = reMarkup.Replace(context, "", -1, -1)
	numbers := map[string]bool{}
	for _, m := range matches(reGrouped, context) {
		numbers[m[2]] = true
	}
	for _, m := range matches(reShort, context) {
		numbers[m[1]] = true
	}
	for _, m := range matches(reRepeated, context) {
		if m[1] == "." {
			numbers[","] = true
		} else {
			numbers["."] = true
		}
	}
	locales := map[string]bool{}
	for _, m := range matches(reLocale, fold(context)) {
		switch m[1] {
		case "en", "ja", "zh", "ko":
			locales["."] = true
		default:
			locales[","] = true
		}
	}
	for _, evidence := range []map[string]bool{currencyMarks(written), numbers, currencyMarks(context), locales} {
		if len(evidence) > 0 {
			return one(evidence)
		}
	}
	return "", false
}

// ParseNumber reads the written magnitude, percent points included: its
// currency and percent sign set apart, its scale word applied; no implicit
// unit and no conversion. ok is false when the writing does not settle one
// value. The value is an int64 when it is whole, else a float64.
func ParseNumber(text, context, unit string) (value any, ok bool) {
	if unit != "" && isMatch(reCurrencyFull, strings.TrimSpace(unit)) {
		context += "\n" + unit
	}
	written := text
	text = strings.TrimSpace(text)
	text = strings.NewReplacer("−", "-", " ", " ", " ", " ").Replace(text)
	text = strings.TrimRightFunc(strings.TrimSuffix(text, "%"), unicode.IsSpace)
	negative := strings.HasPrefix(text, "(") && strings.HasSuffix(text, ")") && len(text) >= 2
	if negative {
		text = strings.TrimSpace(text[1 : len(text)-1])
	}
	sign, currency := "", ""
	for range 2 { // one currency and one sign, in either order
		if currency == "" {
			if m := first(reCurrencySplit, text); m != nil {
				if m[1] != "" {
					currency, text = m[1], strings.TrimSpace(m[2])
				} else {
					currency, text = m[4], strings.TrimSpace(m[3])
				}
			}
		}
		if sign == "" && (strings.HasPrefix(text, "-") || strings.HasPrefix(text, "+")) {
			sign, text = text[:1], strings.TrimSpace(text[1:])
		}
	}
	if negative && sign != "" {
		return nil, false
	}
	scale := int64(1)
	if m := first(reScale, text); m != nil {
		word := m[1]
		if word == "Billion" {
			return nil, false
		}
		scale = scales[strings.ToLower(word)]
		text = strings.TrimSuffix(text, m[0])
	} else if m := first(reSuffix, text); m != nil {
		if !isMatch(reCurrency, written+"\n"+context) && !isMatch(reCounted, context) {
			return nil, false
		}
		scale = map[string]int64{"k": 1e3, "m": 1e6, "b": 1e9, "bn": 1e9}[strings.ToLower(m[1])]
		text = strings.TrimSuffix(text, m[0])
	}
	digits, ok := parseDigits(text, written, context)
	if !ok || scale == 0 {
		return nil, false
	}
	if negative {
		sign = "-"
	}
	r, ok := new(big.Rat).SetString(sign + digits)
	if !ok {
		return nil, false
	}
	r.Mul(r, new(big.Rat).SetInt64(scale))
	if r.IsInt() && r.Num().IsInt64() {
		return r.Num().Int64(), true
	}
	f, _ := r.Float64()
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return nil, false
	}
	return f, true
}

// parseDigits is text's number as plain digits and a point: the decimal mark
// from the evidence, the other mark a complete grouping (threes, or India's
// lakh grouping beside a rupee).
func parseDigits(text, written, context string) (string, bool) {
	if fullMatch(reDigitsOnly, text) == nil {
		return "", false
	}
	if strings.ContainsAny(text, " '’") { // a space or an apostrophe only groups
		g := fullMatch(reSpacedGroup, text)
		if g == nil {
			return "", false
		}
		plain := strings.NewReplacer(" ", "", "'", "", "’", "").Replace(g[1])
		return parseDigits(plain+g[2], written, context)
	}
	if fullMatch(reLakh, text) != nil && isMatch(reRupee, written+"\n"+context) {
		return strings.ReplaceAll(text, ",", ""), true
	}
	if strings.Contains(text, ".") && strings.Contains(text, ",") {
		point, group := ",", "."
		if strings.LastIndex(text, ".") > strings.LastIndex(text, ",") {
			point, group = ".", ","
		}
		re := mustRE(`\A\d{1,3}(?:` + escape(group) + `\d{3})+` + escape(point) + `\d+\z`)
		if !isMatch(re, text) {
			return "", false
		}
		return strings.ReplaceAll(strings.ReplaceAll(text, group, ""), point, "."), true
	}
	if !strings.Contains(text, ".") && !strings.Contains(text, ",") {
		return text, true
	}
	mark := ","
	if strings.Contains(text, ".") {
		mark = "."
	}
	parts := strings.Split(text, mark)
	for _, p := range parts {
		if p == "" {
			return "", false
		}
	}
	point, settled := decimalMark(written, context)
	if len(parts) > 2 || settled && point != mark { // the mark groups: complete threes
		if len(parts[0]) < 1 || len(parts[0]) > 3 {
			return "", false
		}
		for _, p := range parts[1:] {
			if len(p) != 3 {
				return "", false
			}
		}
		return strings.Join(parts, ""), true
	}
	if !settled && len(parts[1]) == 3 && len(parts[0]) <= 3 { // 1,250: a group or a decimal, unsettled
		return "", false
	}
	return strings.Join(parts, "."), true
}

// dateOrder is the declared format or locale first, then the state's other
// unambiguous numeric dates; conflicts stay unresolved.
func dateOrder(context, folded string) (string, bool) {
	explicit := map[string]bool{}
	for _, f := range dateFormats {
		if isMatch(f.re, folded) {
			explicit[f.order] = true
		}
	}
	if len(explicit) > 0 {
		return one(explicit)
	}
	if locales := matches(reDateLocale, folded); len(locales) > 0 {
		set := map[string]bool{}
		for _, m := range locales {
			l := strings.ReplaceAll(m[1], "_", "-")
			if l == "en" || l == "en-us" {
				set["mdy"] = true
			} else {
				set["dmy"] = true
			}
		}
		return one(set)
	}
	orders := map[string]bool{}
	for _, m := range matches(reNumericDate, context) {
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[3])
		year, _ := strconv.Atoi(m[4])
		if len(m[4]) != 4 || a > 31 || b > 31 || a < 1 || b < 1 {
			continue
		}
		for _, c := range []struct {
			day, month int
			order      string
		}{{a, b, "dmy"}, {b, a, "mdy"}} {
			if !validDate(year, c.month, c.day) {
				continue
			}
			if (a > 12) != (b > 12) {
				orders[c.order] = true
			}
		}
	}
	return one(orders)
}

func validDate(year, month, day int) bool {
	if year < 1 || year > 9999 || month < 1 || month > 12 || day < 1 {
		return false
	}
	return day <= time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func iso(year, month, day int) string { return fmt.Sprintf("%04d-%02d-%02d", year, month, day) }

type dateContext struct {
	context      string
	folded       string
	foldedDone   bool
	order        string
	orderSettled bool
	orderDone    bool
}

func (c *dateContext) fold() string {
	if !c.foldedDone {
		c.folded, c.foldedDone = foldAccents(c.context), true
	}
	return c.folded
}

func (c *dateContext) dateOrder() (string, bool) {
	if !c.orderDone {
		c.order, c.orderSettled = dateOrder(c.context, c.fold())
		c.orderDone = true
	}
	return c.order, c.orderSettled
}

// ParseDate reads an ISO date (or date-time, when a time is written) and
// whether the reading stayed ambiguous; an invalid or unrecognised span is
// ("", false).
func ParseDate(text, context string) (string, bool) {
	return (&dateContext{context: context}).parse(text)
}

func (c *dateContext) candidates(year, a, b int) [][3]int {
	dmy, mdy := [3]int{year, b, a}, [3]int{year, a, b}
	order, settled := c.dateOrder()
	if !settled {
		return [][3]int{dmy, mdy}
	}
	if order == "dmy" {
		return [][3]int{dmy}
	}
	return [][3]int{mdy}
}

func (c *dateContext) parse(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	// Keep an explicitly written time and timezone, with no default time or timezone.
	if m := first(reTimed, text); m != nil {
		date, ambiguous := c.parse(m[1])
		if date == "" {
			return "", ambiguous
		}
		clock := m[2]
		if clock[1] == ':' {
			clock = "0" + clock
		}
		return isoDateTime(date, clock)
	}
	if m := fullMatch(reNumericDate, text); m != nil {
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[3])
		cc, _ := strconv.Atoi(m[4])
		var cands [][3]int
		switch {
		case len(m[1]) == 4:
			cands = [][3]int{{a, b, cc}}
		case len(m[4]) == 4:
			cands = c.candidates(cc, a, b)
		default:
			// A century is never invented: another unambiguous complete date must establish it.
			if len(m[4]) != 2 {
				return "", false
			}
			centuries := map[int]bool{}
			var complete []string
			for _, n := range matches(reNumericDate, c.context) {
				if len(n[1]) == 4 || len(n[4]) == 4 {
					complete = append(complete, n[0])
				}
			}
			for _, w := range matches(reWordDate, c.fold()) {
				complete = append(complete, w[0])
			}
			for _, cand := range complete {
				value, ambiguous := c.parse(cand)
				if value != "" && !ambiguous {
					y, _ := strconv.Atoi(value[:4])
					centuries[y/100*100] = true
				}
			}
			if len(centuries) != 1 {
				return "", false
			}
			for century := range centuries {
				cands = c.candidates(century+cc, a, b)
			}
		}
		values := map[string]bool{}
		for _, d := range cands {
			if validDate(d[0], d[1], d[2]) {
				values[iso(d[0], d[1], d[2])] = true
			}
		}
		if len(values) > 1 {
			return "", true
		}
		v, _ := one(values)
		return v, false
	}
	if m := fullMatch(reCJKDate, text); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		if !validDate(y, mo, d) {
			return "", false
		}
		return iso(y, mo, d), false
	}
	words := foldAccents(text)
	words, _ = reOrdinal.Replace(words, "", -1, -1)
	words, _ = reDegree.Replace(words, "", -1, -1)
	words, _ = reArticle.Replace(words, " ", -1, -1)
	fields := strings.Fields(strings.NewReplacer(",", " ", ".", " ").Replace(words))
	if len(fields) > 0 && weekdays[strings.TrimSuffix(fields[0], "-feira")] {
		fields = fields[1:]
	}
	if len(fields) != 3 {
		return "", false
	}
	var day, year string
	var month int
	if m, ok := months[fields[0]]; ok {
		month, day, year = m, fields[1], fields[2]
	} else if m, ok := months[fields[1]]; ok {
		day, month, year = fields[0], m, fields[2]
	} else {
		return "", false
	}
	if !allDigits(day) || !allDigits(year) || len(year) != 4 {
		return "", false
	}
	d, _ := strconv.Atoi(day)
	y, _ := strconv.Atoi(year)
	if !validDate(y, month, d) {
		return "", false
	}
	return iso(y, month, d), false
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// isoDateTime is Python's datetime.fromisoformat(date + "T" + clock).isoformat().
func isoDateTime(date, clock string) (string, bool) {
	re := mustRE(`\A(\d{2}):(\d{2})(?::(\d{2})(?:\.(\d+))?)?(Z|[+-]\d{2}:\d{2})?\z`)
	m := first(re, clock)
	if m == nil {
		return "", false
	}
	h, _ := strconv.Atoi(m[1])
	mi, _ := strconv.Atoi(m[2])
	s, _ := strconv.Atoi(m[3])
	if h > 23 || mi > 59 || s > 59 {
		return "", false
	}
	out := fmt.Sprintf("%sT%02d:%02d:%02d", date, h, mi, s)
	if frac := m[4]; frac != "" {
		// Python 3.11+ reads any number of digits and keeps microseconds.
		frac = (frac + "000000")[:6]
		if frac != "000000" {
			out += "." + frac
		}
	}
	switch tz := m[5]; {
	case tz == "Z":
		out += "+00:00"
	case tz != "":
		th, _ := strconv.Atoi(tz[1:3])
		tm, _ := strconv.Atoi(tz[4:6])
		if th > 23 || tm > 59 {
			return "", false
		}
		out += tz
	}
	return out, false
}
