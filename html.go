// Go port of the subset of Sundown (the C library behind Redcarpet::Render::HTML) that
// to_html.rb relied on: default extensions, default render flags. Function names follow
// markdown.c / html.c so the two can be read side by side.
//
// Not ported (never produced by changelog.go): block-level raw HTML, blockquotes,
// reference-style link definitions, tables, fenced code.
package main

import (
	"bytes"
	"fmt"
	"strings"
)

func isAlnum(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isSpace(c byte) bool { return c == ' ' || c == '\n' }

// ---------------------------------------------------------------- houdini escaping

func escapeHTML(ob *bytes.Buffer, src []byte) {
	for _, c := range src {
		switch c {
		case '"':
			ob.WriteString("&quot;")
		case '&':
			ob.WriteString("&amp;")
		case '\'':
			ob.WriteString("&#39;")
		case '<':
			ob.WriteString("&lt;")
		case '>':
			ob.WriteString("&gt;")
		default:
			ob.WriteByte(c)
		}
	}
}

const hrefSafe = "-_.+!*'(),%#@?=;:/&$"

func escapeHref(ob *bytes.Buffer, src []byte) {
	for _, c := range src {
		switch {
		case isAlnum(c) || (c < 0x80 && strings.IndexByte(hrefSafe, c) >= 0 && c != '\''):
			ob.WriteByte(c)
		case c == '\'':
			ob.WriteString("&#x27;")
		default:
			fmt.Fprintf(ob, "%%%02X", c)
		}
	}
}

// ---------------------------------------------------------------- inline parsing

type mdRenderer struct{}

func (r *mdRenderer) parseInline(ob *bytes.Buffer, data []byte) {
	i, end, consumed := 0, 0, 0
	size := len(data)
	for i < size {
		for end < size && !isActiveChar(data[end]) {
			end++
		}
		escapeHTML(ob, data[i:end]) // normal_text
		if end >= size {
			break
		}
		i = end
		n := r.charTrigger(ob, data, i, i-consumed)
		if n == 0 {
			end = i + 1
		} else {
			i += n
			end = i
			consumed = i
		}
	}
}

func isActiveChar(c byte) bool {
	switch c {
	case '*', '_', '`', '\n', '[', '<', '\\', '&':
		return true
	}
	return false
}

// charTrigger dispatches on data[i]; offset is the distance to the last consumed position.
func (r *mdRenderer) charTrigger(ob *bytes.Buffer, data []byte, i, offset int) int {
	switch data[i] {
	case '*', '_':
		return r.charEmphasis(ob, data, i)
	case '`':
		return r.charCodespan(ob, data[i:])
	case '\n':
		return r.charLinebreak(ob, data, i, offset)
	case '[':
		return r.charLink(ob, data, i, offset)
	case '<':
		return r.charLangleTag(ob, data[i:])
	case '\\':
		return r.charEscape(ob, data[i:])
	case '&':
		return r.charEntity(ob, data[i:])
	}
	return 0
}

func findEmphChar(data []byte, c byte) int {
	size := len(data)
	i := 1
	for i < size {
		for i < size && data[i] != c && data[i] != '[' {
			i++
		}
		if i == size {
			return 0
		}
		if i > 0 && data[i-1] == '\\' {
			i++
			continue
		}
		if data[i] == c {
			return i
		}
		if data[i] == '[' {
			tmpI := 0
			i++
			for i < size && data[i] != ']' {
				if tmpI == 0 && data[i] == c {
					tmpI = i
				}
				i++
			}
			i++
			for i < size && (data[i] == ' ' || data[i] == '\n') {
				i++
			}
			if i >= size {
				return tmpI
			}
			var cc byte
			switch data[i] {
			case '[':
				cc = ']'
			case '(':
				cc = ')'
			default:
				if tmpI != 0 {
					return tmpI
				}
				continue
			}
			i++
			for i < size && data[i] != cc {
				if tmpI == 0 && data[i] == c {
					tmpI = i
				}
				i++
			}
			if i >= size {
				return tmpI
			}
			i++
		}
	}
	return 0
}

// parseEmph1 works on full[start:] so callers can rewind (data - 2 in the C code).
func (r *mdRenderer) parseEmph1(ob *bytes.Buffer, full []byte, start int, c byte) int {
	data := full[start:]
	size := len(data)
	i := 0
	if size > 1 && data[0] == c && data[1] == c {
		i = 1
	}
	for i < size {
		n := findEmphChar(data[i:], c)
		if n == 0 {
			return 0
		}
		i += n
		if i >= size {
			return 0
		}
		if data[i] == c && !isSpace(data[i-1]) {
			var work bytes.Buffer
			r.parseInline(&work, data[:i])
			if work.Len() == 0 {
				return 0
			}
			ob.WriteString("<em>")
			ob.Write(work.Bytes())
			ob.WriteString("</em>")
			return i + 1
		}
	}
	return 0
}

func (r *mdRenderer) parseEmph2(ob *bytes.Buffer, full []byte, start int, c byte) int {
	data := full[start:]
	size := len(data)
	i := 0
	for i < size {
		n := findEmphChar(data[i:], c)
		if n == 0 {
			return 0
		}
		i += n
		if i+1 < size && data[i] == c && data[i+1] == c && i > 0 && !isSpace(data[i-1]) {
			var work bytes.Buffer
			r.parseInline(&work, data[:i])
			if work.Len() == 0 {
				return 0
			}
			ob.WriteString("<strong>")
			ob.Write(work.Bytes())
			ob.WriteString("</strong>")
			return i + 2
		}
		i++
	}
	return 0
}

func (r *mdRenderer) parseEmph3(ob *bytes.Buffer, full []byte, start int, c byte) int {
	data := full[start:]
	size := len(data)
	i := 0
	for i < size {
		n := findEmphChar(data[i:], c)
		if n == 0 {
			return 0
		}
		i += n
		if data[i] != c || isSpace(data[i-1]) {
			continue
		}
		if i+2 < size && data[i+1] == c && data[i+2] == c {
			var work bytes.Buffer
			r.parseInline(&work, data[:i])
			if work.Len() == 0 {
				return 0
			}
			ob.WriteString("<strong><em>")
			ob.Write(work.Bytes())
			ob.WriteString("</em></strong>")
			return i + 3
		} else if i+1 < size && data[i+1] == c {
			n = r.parseEmph1(ob, full, start-2, c)
			if n == 0 {
				return 0
			}
			return n - 2
		} else {
			n = r.parseEmph2(ob, full, start-1, c)
			if n == 0 {
				return 0
			}
			return n - 1
		}
	}
	return 0
}

func (r *mdRenderer) charEmphasis(ob *bytes.Buffer, full []byte, at int) int {
	data := full[at:]
	size := len(data)
	c := data[0]
	if size > 2 && data[1] != c {
		if isSpace(data[1]) {
			return 0
		}
		if n := r.parseEmph1(ob, full, at+1, c); n != 0 {
			return n + 1
		}
		return 0
	}
	if size > 3 && data[1] == c && data[2] != c {
		if isSpace(data[2]) {
			return 0
		}
		if n := r.parseEmph2(ob, full, at+2, c); n != 0 {
			return n + 2
		}
		return 0
	}
	if size > 4 && data[1] == c && data[2] == c && data[3] != c {
		if isSpace(data[3]) {
			return 0
		}
		if n := r.parseEmph3(ob, full, at+3, c); n != 0 {
			return n + 3
		}
		return 0
	}
	return 0
}

func (r *mdRenderer) charLinebreak(ob *bytes.Buffer, data []byte, at, offset int) int {
	if offset < 2 || data[at-1] != ' ' || data[at-2] != ' ' {
		return 0
	}
	b := ob.Bytes()
	n := len(b)
	for n > 0 && b[n-1] == ' ' {
		n--
	}
	ob.Truncate(n)
	ob.WriteString("<br>\n")
	return 1
}

func (r *mdRenderer) charCodespan(ob *bytes.Buffer, data []byte) int {
	size := len(data)
	nb := 0
	for nb < size && data[nb] == '`' {
		nb++
	}
	i, end := 0, nb
	for ; end < size && i < nb; end++ {
		if data[end] == '`' {
			i++
		} else {
			i = 0
		}
	}
	if i < nb && end >= size {
		return 0
	}
	fBegin := nb
	for fBegin < end && data[fBegin] == ' ' {
		fBegin++
	}
	fEnd := end - nb
	for fEnd > nb && data[fEnd-1] == ' ' {
		fEnd--
	}
	ob.WriteString("<code>")
	if fBegin < fEnd {
		escapeHTML(ob, data[fBegin:fEnd])
	}
	ob.WriteString("</code>")
	return end
}

func (r *mdRenderer) charEscape(ob *bytes.Buffer, data []byte) int {
	const escapeChars = "\\`*_{}[]()#+-.!:|&<>^~="
	if len(data) > 1 {
		if strings.IndexByte(escapeChars, data[1]) < 0 {
			return 0
		}
		escapeHTML(ob, data[1:2])
	} else if len(data) == 1 {
		ob.WriteByte(data[0])
	}
	return 2
}

func (r *mdRenderer) charEntity(ob *bytes.Buffer, data []byte) int {
	size := len(data)
	end := 1
	if end < size && data[end] == '#' {
		end++
	}
	for end < size && isAlnum(data[end]) {
		end++
	}
	if end < size && data[end] == ';' {
		end++
	} else {
		return 0
	}
	ob.Write(data[:end])
	return end
}

func isMailAutolink(data []byte) int {
	nb := 0
	for i, c := range data {
		if isAlnum(c) {
			continue
		}
		switch c {
		case '@':
			nb++
		case '-', '.', '_':
		case '>':
			if nb == 1 {
				return i + 1
			}
			return 0
		default:
			return 0
		}
	}
	return 0
}

// tagLength returns the length of the tag at data[0] and whether it is an autolink.
func tagLength(data []byte) (int, bool) {
	size := len(data)
	if size < 3 || data[0] != '<' {
		return 0, false
	}
	i := 1
	if data[1] == '/' {
		i = 2
	}
	if !isAlnum(data[i]) {
		return 0, false
	}
	autolink := false
	for i < size && (isAlnum(data[i]) || data[i] == '.' || data[i] == '+' || data[i] == '-') {
		i++
	}
	if i > 1 && i < size && data[i] == '@' {
		if j := isMailAutolink(data[i:]); j != 0 {
			return i + j, true
		}
	}
	if i > 2 && i < size && data[i] == ':' {
		autolink = true
		i++
	}
	if i >= size {
		autolink = false
	} else if autolink {
		j := i
		for i < size {
			if data[i] == '\\' {
				i += 2
			} else if data[i] == '>' || data[i] == '\'' || data[i] == '"' || data[i] == ' ' || data[i] == '\n' {
				break
			} else {
				i++
			}
		}
		if i >= size {
			return 0, false
		}
		if i > j && data[i] == '>' {
			return i + 1, true
		}
		autolink = false
	}
	for i < size && data[i] != '>' {
		i++
	}
	if i >= size {
		return 0, false
	}
	return i + 1, false
}

func (r *mdRenderer) charLangleTag(ob *bytes.Buffer, data []byte) int {
	end, autolink := tagLength(data)
	if end <= 2 {
		return 0
	}
	if autolink {
		// rndr_autolink, MKDA_NORMAL / MKDA_EMAIL
		link := unscapeText(data[1 : end-1])
		ob.WriteString("<a href=\"")
		if bytes.Contains(link, []byte("@")) && !bytes.Contains(link, []byte(":")) {
			ob.WriteString("mailto:")
		}
		escapeHref(ob, link)
		ob.WriteString("\">")
		if bytes.HasPrefix(link, []byte("mailto:")) {
			escapeHTML(ob, link[7:])
		} else {
			escapeHTML(ob, link)
		}
		ob.WriteString("</a>")
		return end
	}
	ob.Write(data[:end]) // raw_html_tag
	return end
}

func unscapeText(src []byte) []byte {
	var ob bytes.Buffer
	i := 0
	for i < len(src) {
		org := i
		for i < len(src) && src[i] != '\\' {
			i++
		}
		ob.Write(src[org:i])
		if i+1 >= len(src) {
			break
		}
		ob.WriteByte(src[i+1])
		i += 2
	}
	return ob.Bytes()
}

func (r *mdRenderer) charLink(ob *bytes.Buffer, full []byte, at, offset int) int {
	data := full[at:]
	size := len(data)
	isImg := offset > 0 && full[at-1] == '!'
	i := 1
	level := 1
	for ; i < size; i++ {
		if data[i] == '\n' {
			continue
		} else if data[i-1] == '\\' {
			continue
		} else if data[i] == '[' {
			level++
		} else if data[i] == ']' {
			level--
			if level <= 0 {
				break
			}
		}
	}
	if i >= size {
		return 0
	}
	txtE := i
	i++
	for i < size && isSpace(data[i]) {
		i++
	}
	var link, title []byte
	if i < size && data[i] == '(' {
		i++
		for i < size && isSpace(data[i]) {
			i++
		}
		linkB := i
		nbP := 0
		for i < size {
			if data[i] == '\\' {
				i += 2
			} else if data[i] == '(' && i != 0 {
				nbP++
				i++
			} else if data[i] == ')' {
				if nbP == 0 {
					break
				}
				nbP--
				i++
			} else if i >= 1 && isSpace(data[i-1]) && (data[i] == '\'' || data[i] == '"') {
				break
			} else {
				i++
			}
		}
		if i >= size {
			return 0
		}
		linkE := i
		titleB, titleE := 0, 0
		if data[i] == '\'' || data[i] == '"' {
			qtype := data[i]
			inTitle := true
			i++
			titleB = i
			for i < size {
				if data[i] == '\\' {
					i += 2
				} else if data[i] == qtype {
					inTitle = false
					i++
				} else if data[i] == ')' && !inTitle {
					break
				} else {
					i++
				}
			}
			if i >= size {
				return 0
			}
			titleE = i - 1
			for titleE > titleB && isSpace(data[titleE]) {
				titleE--
			}
			if data[titleE] != '\'' && data[titleE] != '"' {
				titleB, titleE = 0, 0
				linkE = i
			}
		}
		for linkE > linkB && isSpace(data[linkE-1]) {
			linkE--
		}
		if linkB < size && data[linkB] == '<' {
			linkB++
		}
		if linkE > 0 && data[linkE-1] == '>' {
			linkE--
		}
		if linkE > linkB {
			link = data[linkB:linkE]
		}
		if titleE > titleB {
			title = data[titleB:titleE]
		}
		i++
	} else {
		// reference-style links need link definitions, which are not supported
		return 0
	}

	var content bytes.Buffer
	if txtE > 1 {
		if isImg {
			content.Write(data[1:txtE])
		} else {
			r.parseInline(&content, data[1:txtE])
		}
	}
	uLink := unscapeText(link)
	if isImg {
		b := ob.Bytes()
		if len(b) > 0 && b[len(b)-1] == '!' {
			ob.Truncate(len(b) - 1)
		}
		ob.WriteString("<img src=\"")
		escapeHref(ob, uLink)
		ob.WriteString("\" alt=\"")
		escapeHTML(ob, content.Bytes())
		if len(title) > 0 {
			ob.WriteString("\" title=\"")
			escapeHTML(ob, title)
		}
		ob.WriteString("\">")
	} else {
		ob.WriteString("<a href=\"")
		escapeHref(ob, uLink)
		if len(title) > 0 {
			ob.WriteString("\" title=\"")
			escapeHTML(ob, title)
		}
		ob.WriteString("\">")
		ob.Write(content.Bytes())
		ob.WriteString("</a>")
	}
	return i
}

// ---------------------------------------------------------------- block parsing

// isEmpty returns the length of the line (including \n) if it holds only spaces, else 0.
func isEmpty(data []byte) int {
	i := 0
	for ; i < len(data) && data[i] != '\n'; i++ {
		if data[i] != ' ' {
			return 0
		}
	}
	return i + 1
}

func isHrule(data []byte) bool {
	size := len(data)
	if size < 3 {
		return false
	}
	i := 0
	for i < 3 && data[i] == ' ' {
		i++
	}
	if i+2 >= size || (data[i] != '*' && data[i] != '-' && data[i] != '_') {
		return false
	}
	c := data[i]
	n := 0
	for i < size && data[i] != '\n' {
		if data[i] == c {
			n++
		} else if data[i] != ' ' {
			return false
		}
		i++
	}
	return n >= 3
}

func isHeaderline(data []byte) int {
	size := len(data)
	if size == 0 {
		return 0
	}
	if data[0] == '=' || data[0] == '-' {
		c := data[0]
		i := 1
		for i < size && data[i] == c {
			i++
		}
		for i < size && data[i] == ' ' {
			i++
		}
		if i >= size || data[i] == '\n' {
			if c == '=' {
				return 1
			}
			return 2
		}
	}
	return 0
}

func isNextHeaderline(data []byte) bool {
	i := 0
	for i < len(data) && data[i] != '\n' {
		i++
	}
	i++
	if i >= len(data) {
		return false
	}
	return isHeaderline(data[i:]) != 0
}

func skipUpTo3Spaces(data []byte) int {
	i := 0
	for i < 3 && i < len(data) && data[i] == ' ' {
		i++
	}
	return i
}

func prefixOli(data []byte) int {
	size := len(data)
	i := skipUpTo3Spaces(data)
	if i >= size || data[i] < '0' || data[i] > '9' {
		return 0
	}
	for i < size && data[i] >= '0' && data[i] <= '9' {
		i++
	}
	if i+1 >= size || data[i] != '.' || data[i+1] != ' ' {
		return 0
	}
	if isNextHeaderline(data[i:]) {
		return 0
	}
	return i + 2
}

func prefixUli(data []byte) int {
	size := len(data)
	i := skipUpTo3Spaces(data)
	if i+1 >= size || (data[i] != '*' && data[i] != '+' && data[i] != '-') || data[i+1] != ' ' {
		return 0
	}
	if isNextHeaderline(data[i:]) {
		return 0
	}
	return i + 2
}

func (r *mdRenderer) rndrHeader(ob *bytes.Buffer, text []byte, level int) {
	if ob.Len() > 0 {
		ob.WriteByte('\n')
	}
	fmt.Fprintf(ob, "<h%d>", level)
	ob.Write(text)
	fmt.Fprintf(ob, "</h%d>\n", level)
}

func (r *mdRenderer) rndrParagraph(ob *bytes.Buffer, text []byte) {
	if ob.Len() > 0 {
		ob.WriteByte('\n')
	}
	i := 0
	for i < len(text) && (text[i] == ' ' || text[i] == '\n' || text[i] == '\t' || text[i] == '\r' || text[i] == '\f' || text[i] == '\v') {
		i++
	}
	if i == len(text) {
		return
	}
	ob.WriteString("<p>")
	ob.Write(text[i:])
	ob.WriteString("</p>\n")
}

func (r *mdRenderer) parseAtxHeader(ob *bytes.Buffer, data []byte) int {
	size := len(data)
	level := 0
	for level < size && level < 6 && data[level] == '#' {
		level++
	}
	i := level
	for i < size && data[i] == ' ' {
		i++
	}
	end := i
	for end < size && data[end] != '\n' {
		end++
	}
	skip := end
	for end > 0 && data[end-1] == '#' {
		end--
	}
	for end > 0 && data[end-1] == ' ' {
		end--
	}
	if end > i {
		var work bytes.Buffer
		r.parseInline(&work, data[i:end])
		r.rndrHeader(ob, work.Bytes(), level)
	}
	return skip
}

func (r *mdRenderer) parseParagraph(ob *bytes.Buffer, data []byte) int {
	size := len(data)
	i, end, level := 0, 0, 0
	lastIsEmpty := true
	for i < size {
		end = i + 1
		for end < size && data[end-1] != '\n' {
			end++
		}
		if isEmpty(data[i:]) != 0 {
			break
		}
		if !lastIsEmpty {
			if level = isHeaderline(data[i:]); level != 0 {
				break
			}
		}
		lastIsEmpty = false
		if data[i] == '#' || isHrule(data[i:]) || prefixQuote(data[i:]) != 0 {
			end = i
			break
		}
		i = end
	}
	workSize := i
	for workSize > 0 && data[workSize-1] == '\n' {
		workSize--
	}
	if level == 0 {
		var tmp bytes.Buffer
		r.parseInline(&tmp, data[:workSize])
		r.rndrParagraph(ob, tmp.Bytes())
		return end
	}
	// setext header: the last line of the paragraph is the header text
	work := data[:workSize]
	if len(work) > 0 {
		i = len(work)
		ws := len(work) - 1
		for ws > 0 && data[ws] != '\n' {
			ws--
		}
		beg := ws + 1
		for ws > 0 && data[ws-1] == '\n' {
			ws--
		}
		if ws > 0 {
			var tmp bytes.Buffer
			r.parseInline(&tmp, data[:ws])
			r.rndrParagraph(ob, tmp.Bytes())
			work = data[beg:i]
		} else {
			work = data[:i]
		}
	}
	var hw bytes.Buffer
	r.parseInline(&hw, work)
	r.rndrHeader(ob, hw.Bytes(), level)
	return end
}

func prefixCode(data []byte) int {
	if len(data) > 3 && data[0] == ' ' && data[1] == ' ' && data[2] == ' ' && data[3] == ' ' {
		return 4
	}
	return 0
}

func (r *mdRenderer) parseBlockcode(ob *bytes.Buffer, data []byte) int {
	size := len(data)
	var work bytes.Buffer
	beg := 0
	for beg < size {
		end := beg + 1
		for end < size && data[end-1] != '\n' {
			end++
		}
		if pre := prefixCode(data[beg:end]); pre != 0 {
			beg += pre
		} else if isEmpty(data[beg:end]) == 0 {
			break
		}
		if beg < end {
			if isEmpty(data[beg:end]) != 0 {
				work.WriteByte('\n')
			} else {
				work.Write(data[beg:end])
			}
		}
		beg = end
	}
	w := work.Bytes()
	for len(w) > 0 && w[len(w)-1] == '\n' {
		w = w[:len(w)-1]
	}
	// rndr_blockcode
	if ob.Len() > 0 {
		ob.WriteByte('\n')
	}
	ob.WriteString("<pre><code>")
	escapeHTML(ob, w)
	ob.WriteString("\n</code></pre>\n")
	return beg
}

func prefixQuote(data []byte) int {
	i := skipUpTo3Spaces(data)
	if i < len(data) && data[i] == '>' {
		if i+1 < len(data) && data[i+1] == ' ' {
			return i + 2
		}
		return i + 1
	}
	return 0
}

const (
	mkdListOrdered = 1
	mkdLiBlock     = 2
	mkdLiEnd       = 4
)

func (r *mdRenderer) parseListItem(ob *bytes.Buffer, data []byte, flags *int) int {
	size := len(data)
	orgpre := 0
	for orgpre < 3 && orgpre < size && data[orgpre] == ' ' {
		orgpre++
	}
	beg := prefixUli(data)
	if beg == 0 {
		beg = prefixOli(data)
	}
	if beg == 0 {
		return 0
	}
	end := beg
	for end < size && data[end-1] != '\n' {
		end++
	}
	var work, inter bytes.Buffer
	work.Write(data[beg:end])
	beg = end
	sublist := 0
	inEmpty, hasInsideEmpty := false, false

	for beg < size {
		end++
		for end < size && data[end-1] != '\n' {
			end++
		}
		if isEmpty(data[beg:end]) != 0 {
			inEmpty = true
			beg = end
			continue
		}
		i := 0
		for i < 4 && beg+i < end && data[beg+i] == ' ' {
			i++
		}
		pre := i
		hasNextUli := prefixUli(data[beg+i : end])
		hasNextOli := prefixOli(data[beg+i : end])

		if inEmpty && (((*flags&mkdListOrdered) != 0 && hasNextUli != 0) || ((*flags&mkdListOrdered) == 0 && hasNextOli != 0)) {
			*flags |= mkdLiEnd
			break
		}
		if (hasNextUli != 0 && !isHrule(data[beg+i:end])) || hasNextOli != 0 {
			if inEmpty {
				hasInsideEmpty = true
			}
			if pre == orgpre {
				break
			}
			if sublist == 0 {
				sublist = work.Len()
			}
		} else if inEmpty && i < 4 && data[beg] != '\t' {
			*flags |= mkdLiEnd
			break
		} else if inEmpty {
			work.WriteByte('\n')
			hasInsideEmpty = true
		}
		inEmpty = false
		work.Write(data[beg+i : end])
		beg = end
	}

	if hasInsideEmpty {
		*flags |= mkdLiBlock
	}
	w := work.Bytes()
	if (*flags & mkdLiBlock) != 0 {
		if sublist != 0 && sublist < len(w) {
			r.parseBlock(&inter, w[:sublist])
			r.parseBlock(&inter, w[sublist:])
		} else {
			r.parseBlock(&inter, w)
		}
	} else {
		if sublist != 0 && sublist < len(w) {
			r.parseInline(&inter, w[:sublist])
			r.parseBlock(&inter, w[sublist:])
		} else {
			r.parseInline(&inter, w)
		}
	}
	// rndr_listitem
	ob.WriteString("<li>")
	text := inter.Bytes()
	for len(text) > 0 && text[len(text)-1] == '\n' {
		text = text[:len(text)-1]
	}
	ob.Write(text)
	ob.WriteString("</li>\n")
	return beg
}

func (r *mdRenderer) parseList(ob *bytes.Buffer, data []byte, flags int) int {
	var work bytes.Buffer
	i := 0
	for i < len(data) {
		j := r.parseListItem(&work, data[i:], &flags)
		i += j
		if j == 0 || (flags&mkdLiEnd) != 0 {
			break
		}
	}
	// rndr_list
	if ob.Len() > 0 {
		ob.WriteByte('\n')
	}
	open, close := "<ul>\n", "</ul>\n"
	if flags&mkdListOrdered != 0 {
		open, close = "<ol>\n", "</ol>\n"
	}
	ob.WriteString(open)
	ob.Write(work.Bytes())
	ob.WriteString(close)
	return i
}

func (r *mdRenderer) parseBlock(ob *bytes.Buffer, data []byte) {
	size := len(data)
	beg := 0
	for beg < size {
		txt := data[beg:]
		switch {
		case txt[0] == '#':
			beg += r.parseAtxHeader(ob, txt)
		case isEmpty(txt) != 0:
			beg += isEmpty(txt)
		case isHrule(txt):
			if ob.Len() > 0 {
				ob.WriteByte('\n')
			}
			ob.WriteString("<hr>\n")
			for beg < size && data[beg] != '\n' {
				beg++
			}
			beg++
		case prefixCode(txt) != 0:
			beg += r.parseBlockcode(ob, txt)
		case prefixUli(txt) != 0:
			beg += r.parseList(ob, txt, 0)
		case prefixOli(txt) != 0:
			beg += r.parseList(ob, txt, mkdListOrdered)
		default:
			beg += r.parseParagraph(ob, txt)
		}
	}
}

// expandTabs mirrors sundown's preprocessing: tabs become spaces up to the next multiple of 4.
func expandTabs(ob *bytes.Buffer, line []byte) {
	tab := 0
	for _, c := range line {
		if c == '\t' {
			for {
				ob.WriteByte(' ')
				tab++
				if tab%4 == 0 {
					break
				}
			}
		} else {
			ob.WriteByte(c)
			tab++
		}
	}
}

// markdownToHTML renders markdown the way Redcarpet::Markdown.new(Redcarpet::Render::HTML) did.
func markdownToHTML(document string) string {
	doc := []byte(document)
	if bytes.HasPrefix(doc, []byte{0xEF, 0xBB, 0xBF}) {
		doc = doc[3:]
	}
	var text bytes.Buffer
	beg := 0
	for beg < len(doc) {
		end := beg
		for end < len(doc) && doc[end] != '\n' && doc[end] != '\r' {
			end++
		}
		expandTabs(&text, doc[beg:end])
		for end < len(doc) && (doc[end] == '\n' || doc[end] == '\r') {
			if doc[end] == '\n' || (end+1 < len(doc) && doc[end+1] != '\n') {
				text.WriteByte('\n')
			}
			end++
		}
		beg = end
	}
	var ob bytes.Buffer
	if text.Len() > 0 {
		if b := text.Bytes(); b[len(b)-1] != '\n' {
			text.WriteByte('\n')
		}
		(&mdRenderer{}).parseBlock(&ob, text.Bytes())
	}
	return ob.String()
}
