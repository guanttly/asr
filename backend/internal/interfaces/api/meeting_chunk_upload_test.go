package api

import (
	"bytes"
	"os"
	"testing"
)

func TestMeetingChunkUploaderAssemblesInOrder(t *testing.T) {
	u := newMeetingChunkUploader(1, 16)
	session, err := u.init(7, "meeting.wav", "标题", nil, "")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, ok := u.get(session.id, 7); !ok {
		t.Fatalf("session not retrievable")
	}
	if _, ok := u.get(session.id, 8); ok {
		t.Fatalf("session retrievable by wrong user")
	}

	if err := u.appendChunk(session, 0, bytes.NewReader([]byte("hello "))); err != nil {
		t.Fatalf("append 0: %v", err)
	}
	if err := u.appendChunk(session, 1, bytes.NewReader([]byte("world"))); err != nil {
		t.Fatalf("append 1: %v", err)
	}

	path, err := u.complete(session)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("assembled content = %q, want %q", string(data), "hello world")
	}
	if _, ok := u.get(session.id, 7); ok {
		t.Fatalf("session should be removed after complete")
	}
}

func TestMeetingChunkUploaderRejectsOutOfOrder(t *testing.T) {
	u := newMeetingChunkUploader(1, 16)
	session, err := u.init(1, "meeting.wav", "", nil, "")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	t.Cleanup(func() { u.discard(session.id) })

	if err := u.appendChunk(session, 1, bytes.NewReader([]byte("x"))); err == nil {
		t.Fatalf("expected out-of-order rejection")
	}
}

func TestMeetingChunkUploaderEnforcesSessionLimit(t *testing.T) {
	u := newMeetingChunkUploader(1, 0) // 0 -> default 4096MB; use chunk limit instead
	u.maxSessionBytes = 4              // force tiny session cap
	session, err := u.init(1, "meeting.wav", "", nil, "")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	t.Cleanup(func() { u.discard(session.id) })

	if err := u.appendChunk(session, 0, bytes.NewReader([]byte("toolong"))); err == nil {
		t.Fatalf("expected session size limit rejection")
	}
}

func TestMeetingChunkUploaderRejectsUnsupportedExt(t *testing.T) {
	u := newMeetingChunkUploader(1, 16)
	if _, err := u.init(1, "meeting.txt", "", nil, ""); err == nil {
		t.Fatalf("expected unsupported extension rejection")
	}
}
