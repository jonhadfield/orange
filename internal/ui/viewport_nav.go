package ui

// indexFrom returns the first entry in lineOf starting at or below line, or
// -1 when every entry starts above it.
func indexFrom(lineOf []int, line int) int {
	for i, l := range lineOf {
		if l >= line {
			return i
		}
	}
	return -1
}

// indexUpTo is indexFrom from the other end: the last entry starting at or
// above line, or -1 when every entry starts below it.
func indexUpTo(lineOf []int, line int) int {
	for i := len(lineOf) - 1; i >= 0; i-- {
		if lineOf[i] <= line {
			return i
		}
	}
	return -1
}

// indexInView returns the first entry whose start line is on screen
// [top, bottom), or -1 when none is.
func indexInView(lineOf []int, top, bottom int) int {
	for i, l := range lineOf {
		if l >= top {
			if l < bottom {
				return i
			}
			break // sorted, so nothing later is in view either
		}
	}
	return -1
}

// cursorLineOffScreen reports whether the selected entry has been scrolled
// out of the viewport, which is what free scrolling does on purpose.
func cursorLineOffScreen(lineOf []int, cursor, yOffset, height int) bool {
	if cursor >= len(lineOf) {
		return false
	}
	line := lineOf[cursor]
	return line < yOffset || line >= yOffset+height
}
