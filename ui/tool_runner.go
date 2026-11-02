package ui

import "context"

// toolRunner runs one tool job at a time in the background. Starting a new
// job cancels the previous one.
type toolRunner struct {
	shell   *Shell
	cancel  context.CancelFunc
	running bool
	gen     int
}

// start runs job on a goroutine; done runs on the UI goroutine after it.
func (r *toolRunner) start(job func(ctx context.Context)) {
	r.stop()
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel, r.running = cancel, true
	r.gen++
	gen := r.gen
	go func() {
		defer cancel()
		job(ctx)
		r.shell.svc.App.Post(func() {
			if r.gen == gen {
				r.running, r.cancel = false, nil
			}
		})
	}()
}

// stop cancels the running job.
func (r *toolRunner) stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.running, r.cancel = false, nil
}

// post runs fn on the UI goroutine if the job that calls it is still the
// current one.
func (r *toolRunner) post(fn func()) {
	gen := r.gen
	r.shell.svc.App.Post(func() {
		if r.gen == gen {
			fn()
		}
	})
}
