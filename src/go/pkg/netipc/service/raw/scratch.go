package raw

func ensureClientScratch(buf *[]byte, needed int) []byte {
	if len(*buf) < needed {
		*buf = make([]byte, needed)
	}
	return (*buf)[:needed]
}
