package terminal

import "time"

// sendLocked queues what the operation in progress rendered and wakes the writer.
// Caller MUST hold mu.
func (t *termImpl) sendLocked() {
	buf := &t.output.buf
	if buf.Len() == 0 {
		return
	}
	t.queued = append(t.queued, buf.Bytes()...)
	buf.Reset()
	select {
	case t.wake <- struct{}{}:
	default:
	}
}

// writeLoop is the only writer of terminal output after Init. It returns once Fini
// has queued the restoring sequences and they are written; a terminal that never
// takes them leaves it in its write, and Fini stops waiting.
func (t *termImpl) writeLoop() {
	defer close(t.writerDone)
	for {
		select {
		case <-t.wake:
			t.drain()
		case <-t.quit:
			t.drain()
			return
		}
	}
}

// drain writes what is queued until nothing is, and draws a frame held while a
// write was out once that write returns. The write runs without mu, so whoever
// draws meanwhile finds writeSince set and holds its frame instead of waiting.
func (t *termImpl) drain() {
	t.mu.Lock()
	for {
		if len(t.queued) == 0 {
			t.releaseLocked()
		}
		if len(t.queued) == 0 {
			break
		}
		t.writing, t.queued = t.queued, t.writing[:0]
		t.writeSince.Store(time.Now().UnixNano())
		t.mu.Unlock()
		_ = t.backend.Write(t.writing) // a failed write is lost, as one through a buffered writer was
		t.writeSince.Store(0)
		t.mu.Lock()
	}
	select {
	case t.drained <- struct{}{}:
	default:
	}
	t.mu.Unlock()
}
