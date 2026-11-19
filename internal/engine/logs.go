package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/forge"
	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

type LogsView struct {
	Step      string `json:"step"`
	Offset    int64  `json:"offset"`
	Next      int64  `json:"next"`
	Bytes     int64  `json:"bytes"`
	Limit     int    `json:"limit"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

var logLock sync.Mutex

func validStep(step string) bool {
	return has([]string{"review", "repair", "sync", "test", "docs", "lint", "push", "pr", "ci"}, step)
}

func appendLog(run, step string, data []byte) (retErr error) {
	if run == "" || !validStep(step) {
		return
	}
	h, err := Home()
	if err != nil {
		return
	}
	logLock.Lock()
	defer logLock.Unlock()
	dir := filepath.Join(h, "logs", run)
	if os.MkdirAll(dir, 0700) != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, step+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer func() { retErr = errors.Join(retErr, f.Close()) }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() >= transcriptLimit {
		return nil
	}
	data = Redact(data)
	if room := int64(transcriptLimit) - info.Size(); int64(len(data)) > room {
		data = data[:room]
	}
	_, retErr = f.Write(data)
	return retErr
}

func readLogs(run string, q Request) (*LogsView, error) {
	if !validStep(q.Step) {
		return nil, errors.New("step must be review, repair, sync, test, docs, lint, push, pr or ci")
	}
	if q.Offset != nil && *q.Offset < 0 {
		return nil, errors.New("offset must be non-negative")
	}
	h, err := Home()
	if err != nil {
		return nil, err
	}
	logLock.Lock()
	defer logLock.Unlock()
	f, err := os.Open(filepath.Join(h, "logs", run, q.Step+".log"))
	var size int64
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		defer func() { _ = f.Close() }() // Closing a read-only log is best-effort after reading.
		info, statErr := f.Stat()
		if statErr != nil {
			return nil, statErr
		}
		size = info.Size()
	}
	offset := int64(0)
	if q.Offset != nil {
		offset = *q.Offset
	}
	if offset > size {
		return nil, errors.New("offset exceeds log length")
	}
	if !q.Full && !q.Follow && q.Offset == nil {
		offset = max(0, size-16384)
	}
	end := min(size, offset+65536)
	if q.Full {
		end = size
	}
	data := make([]byte, end-offset)
	if len(data) > 0 {
		if _, err := f.ReadAt(data, offset); err != nil {
			return nil, err
		}
	}
	return &LogsView{Step: q.Step, Offset: offset, Next: end, Bytes: size, Limit: transcriptLimit, Content: string(data), Truncated: size >= transcriptLimit}, nil
}

type synchronizedWriter struct {
	lock   *sync.Mutex
	writer io.Writer
}

func (w synchronizedWriter) Write(p []byte) (int, error) {
	w.lock.Lock()
	defer w.lock.Unlock()
	return w.writer.Write(p)
}

type logWriter struct {
	output    io.Writer
	run, step string
	pending   []byte
	discard   bool
}

func (w *logWriter) Write(data []byte) (int, error) {
	n := len(data)
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		end := len(data)
		if i >= 0 {
			end = i + 1
		}
		chunk := data[:end]
		data = data[end:]
		if !w.discard {
			if len(w.pending)+len(chunk) > transcriptLimit {
				w.pending = nil
				w.discard = true
			} else {
				w.pending = append(w.pending, chunk...)
			}
		}
		if i >= 0 {
			if err := w.flush(); err != nil {
				return n - len(data), err
			}
		}
	}
	return n, nil
}
func (w *logWriter) flush() error {
	data := w.pending
	if w.discard {
		data = []byte("[oversized output line omitted]\n")
	}
	if w.output != nil {
		if _, err := w.output.Write(Redact(data)); err != nil {
			return err
		}
	} else if len(data) > 0 {
		_ = appendLog(w.run, w.step, data) // Logs are supplementary evidence; workflow output remains authoritative.
	}
	w.pending = nil
	w.discard = false
	return nil
}

type stageLogKey struct{}
type checkProgressKey struct{}

func observeProcesses(ctx context.Context, run, step string, emit func(FeedEvent)) context.Context {
	ctx = context.WithValue(ctx, stageLogKey{}, func(data []byte) { _ = appendLog(run, step, data) })
	ctx = context.WithValue(ctx, checkProgressKey{}, func(checks []forge.CheckState) {
		emit(FeedEvent{Run: run, Stage: step, Kind: "checks", Checks: checks})
	})
	return runner.WithObserver(ctx, processObserver(run, step, emit))
}

func processObserver(run, step string, emit func(FeedEvent)) runner.ProcessObserver {
	return func(command []string, stdout, stderr io.Writer) (io.Writer, io.Writer, func(int, error, time.Duration)) {
		safe := make([]string, len(command))
		for i, arg := range command {
			safe[i] = string(Redact([]byte(arg)))
		}
		id := opaque()
		emit(FeedEvent{Run: run, Stage: step, Kind: "step-started", Execution: id, Command: safe})
		header, _ := json.Marshal(safe)
		_ = appendLog(run, step, append(append([]byte("=== "), header...), '\n'))
		out, errOut := &logWriter{run: run, step: step}, &logWriter{run: run, step: step}
		if stdout == nil {
			stdout = io.Discard
		}
		if stderr == nil {
			stderr = io.Discard
		}
		lock := &sync.Mutex{}
		return synchronizedWriter{lock, io.MultiWriter(stdout, out)}, synchronizedWriter{lock, io.MultiWriter(stderr, errOut)}, func(code int, executionErr error, duration time.Duration) {
			_ = out.flush()
			_ = errOut.flush()
			elapsed := duration.Milliseconds()
			kind := "step-finished"
			if code != 0 || executionErr != nil {
				kind = "step-failed"
			}
			_ = appendLog(run, step, []byte(fmt.Sprintf("exit=%d elapsed_ms=%d\n", code, elapsed)))
			emit(FeedEvent{Run: run, Stage: step, Kind: kind, Execution: id, Command: safe, ExitCode: &code, ElapsedMS: &elapsed})
		}
	}
}

func logStageResult(run string, stage StageResult) {
	b, _ := json.Marshal(stage)
	_ = appendLog(run, stage.Name, append(b, '\n'))
}

func logsComplete(r *Run, step string) bool {
	if r == nil {
		return true
	}
	if r.Status != "running" && r.Status != "writing" && r.Status != "cancelling" {
		return true
	}
	if r.Pipeline != nil && r.Pipeline.Active != nil {
		active := r.Pipeline.Active
		return active.Stage != step && (step != "repair" || (active.Kind != "repair" && active.Kind != "conflict"))
	}
	switch r.Stage {
	case "independent-review":
		return step != "review"
	case "selected-repair":
		return step != "repair"
	default:
		return true
	}
}
