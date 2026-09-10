package tui

// listMore counts items hidden above and below a visible list window.
type listMore struct {
	above int
	below int
}

// listWindow picks the [start, end) slice of a list to display, keeping the
// cursor on-screen within maxRows visible items. When maxRows is 0 or the list
// fits, returns the full range and zero "more" counters.
func listWindow(total, cursor, maxRows int) (start, end int, more listMore) {
	if total == 0 {
		return 0, 0, listMore{}
	}
	rows := total
	if maxRows > 0 {
		rows = maxRows
		if rows < 3 {
			rows = 3
		}
	}
	if rows >= total {
		return 0, total, listMore{}
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= total {
		cursor = total - 1
	}
	start = cursor - rows/2
	if start < 0 {
		start = 0
	}
	end = start + rows
	if end > total {
		end = total
		start = end - rows
		if start < 0 {
			start = 0
		}
	}
	more.above = start
	more.below = total - end
	return start, end, more
}
