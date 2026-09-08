package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"cloudrail/internal/deployment"
)

type fakeRuntime struct {
	calls      *[]string
	pullErr    error
	runningErr error
	runningID  string
}

func (f fakeRuntime) Pull(context.Context, deployment.Deployment) error {
	*f.calls = append(*f.calls, "pull")
	return f.pullErr
}
func (f fakeRuntime) Ensure(context.Context, deployment.Deployment) error {
	*f.calls = append(*f.calls, "ensure")
	return nil
}
func (f fakeRuntime) Stop(_ context.Context, id string) error {
	*f.calls = append(*f.calls, "stop:"+id)
	return nil
}
func (f fakeRuntime) Remove(_ context.Context, id string) error {
	*f.calls = append(*f.calls, "remove:"+id)
	return nil
}
func (f fakeRuntime) Logs(context.Context, string) string {
	return "token=very-secret-value\ncontainer output"
}
func (f fakeRuntime) Running(_ context.Context, d deployment.Deployment) error {
	*f.calls = append(*f.calls, "running:"+d.ID)
	if f.runningID == "" || f.runningID == d.ID {
		return f.runningErr
	}
	return nil
}

type fakeRoutes struct {
	calls      *[]string
	restoreErr error
}

func (f fakeRoutes) Set(_ deployment.Service, d *deployment.Deployment) error {
	id := "none"
	if d != nil {
		id = d.ID
	}
	*f.calls = append(*f.calls, "route:"+id)
	if id == "old" {
		return f.restoreErr
	}
	return nil
}

type fakeReporter struct {
	calls     *[]string
	reports   []deployment.Report
	activeErr error
}
type fakeCronReporter struct {
	report deployment.Report
	id     string
}

func (f *fakeCronReporter) ReportCronRun(_ context.Context, id string, report deployment.Report) error {
	f.id, f.report = id, report
	return nil
}

type fakeCronRuntime struct {
	fakeRuntime
	exit int
	logs string
	err  error
}

func (f fakeCronRuntime) RunOnce(_ context.Context, _ deployment.Deployment, id string) (int, string, error) {
	*f.calls = append(*f.calls, "cron:"+id)
	return f.exit, f.logs, f.err
}
func (f fakeCronRuntime) RemoveCron(_ context.Context, id string) error {
	*f.calls = append(*f.calls, "remove-cron:"+id)
	return nil
}

func (f *fakeReporter) Report(_ context.Context, _ string, r deployment.Report) error {
	*f.calls = append(*f.calls, "report:"+r.Status)
	f.reports = append(f.reports, r)
	if r.Status == "active" {
		return f.activeErr
	}
	return nil
}
func work() deployment.Work {
	return deployment.Work{Deployment: deployment.Deployment{ID: "new", ServiceID: "service", Port: 80, HealthPath: "/ready"}, Service: deployment.Service{ID: "service", Host: "service.localhost"}, Previous: &deployment.Deployment{ID: "old", HealthPath: "/health"}}
}
func setup(calls *[]string) (*Runner, *fakeReporter) {
	report := &fakeReporter{calls: calls}
	return &Runner{Runtime: fakeRuntime{calls: calls}, Routes: fakeRoutes{calls: calls}, Reporter: report, ProxyURL: "http://proxy", ReadinessTimeout: time.Second, Redact: Redactor(), Check: func(_ context.Context, _ string, _ string, id string) error {
		*calls = append(*calls, "check:"+id)
		return nil
	}}, report
}
func requireOrder(t *testing.T, calls []string, sequence ...string) {
	t.Helper()
	position := 0
	for _, c := range calls {
		if position < len(sequence) && c == sequence[position] {
			position++
		}
	}
	if position != len(sequence) {
		t.Fatalf("missing order %v in %v", sequence, calls)
	}
}
func forbidden(t *testing.T, calls []string, values ...string) {
	t.Helper()
	for _, c := range calls {
		for _, v := range values {
			if c == v {
				t.Fatalf("unexpected %s: %v", v, calls)
			}
		}
	}
}
func TestReplacementActivatesBeforeRetiringOld(t *testing.T) {
	calls := []string{}
	r, p := setup(&calls)
	if err := r.Run(context.Background(), work()); err != nil {
		t.Fatal(err)
	}
	requireOrder(t, calls, "ensure", "check:", "route:new", "check:new", "report:active", "stop:old")
	forbidden(t, calls, "remove:old", "report:failed")
	for _, report := range p.reports {
		if strings.Contains(report.Logs, "very-secret-value") {
			t.Fatal("secret leaked")
		}
	}
}
func TestWorkerActivatesWithoutPublicRoute(t *testing.T) {
	calls := []string{}
	r, _ := setup(&calls)
	w := work()
	w.Service.WorkloadMode = "worker"
	if err := r.Run(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	requireOrder(t, calls, "ensure", "running:new", "route:none", "report:active", "stop:old")
	forbidden(t, calls, "route:new", "route:old", "check:", "check:new")
}
func TestCronReleaseActivatesWithoutStartingAContainer(t *testing.T) {
	calls := []string{}
	r, _ := setup(&calls)
	w := work()
	next := time.Now().Add(time.Minute)
	w.Service.WorkloadMode = "cron"
	w.Service.CronSchedule = "* * * * *"
	w.Service.CronNextRun = &next
	if err := r.Run(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	requireOrder(t, calls, "pull", "report:starting", "route:none", "report:active", "remove:old")
	forbidden(t, calls, "ensure", "route:new", "check:", "check:new", "stop:old")
}

func TestCronRunReportsExitAndLogs(t *testing.T) {
	calls := []string{}
	r, _ := setup(&calls)
	r.Runtime = fakeCronRuntime{fakeRuntime: fakeRuntime{calls: &calls}, exit: 7, logs: "job output"}
	reporter := &fakeCronReporter{}
	w := work()
	w.CronRun = &deployment.CronRun{ID: "run-1"}
	if err := r.RunCron(context.Background(), w, reporter); err != nil {
		t.Fatal(err)
	}
	if reporter.id != "run-1" || reporter.report.Status != "failed" || reporter.report.ExitCode == nil || *reporter.report.ExitCode != 7 || reporter.report.Logs != "job output" {
		t.Fatalf("wrong report: %#v", reporter.report)
	}
	requireOrder(t, calls, "cron:run-1", "remove-cron:run-1")
}
func TestFailedWorkerCandidateRestoresPreviousProcess(t *testing.T) {
	calls := []string{}
	r, _ := setup(&calls)
	r.Runtime = fakeRuntime{calls: &calls, runningErr: errors.New("process exited"), runningID: "new"}
	w := work()
	w.Service.WorkloadMode = "worker"
	if err := r.Run(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	requireOrder(t, calls, "running:new", "route:none", "remove:new", "report:failed")
	forbidden(t, calls, "route:new", "route:old", "stop:old", "remove:old", "check:", "check:new")
}
func TestFailedReadinessPreservesOldRelease(t *testing.T) {
	calls := []string{}
	r, _ := setup(&calls)
	r.Check = func(_ context.Context, _ string, _ string, id string) error {
		calls = append(calls, "check:"+id)
		if id == "" {
			return errors.New("not ready")
		}
		return nil
	}
	if err := r.Run(context.Background(), work()); err != nil {
		t.Fatal(err)
	}
	requireOrder(t, calls, "check:", "route:old", "check:old", "remove:new", "report:failed")
	forbidden(t, calls, "route:new", "stop:old", "report:active")
}
func TestProxyFailureRestoresAndVerifiesBeforeRemovingCandidate(t *testing.T) {
	calls := []string{}
	r, _ := setup(&calls)
	r.Check = func(_ context.Context, _ string, _ string, id string) error {
		calls = append(calls, "check:"+id)
		if id == "new" {
			return errors.New("proxy failed")
		}
		return nil
	}
	if err := r.Run(context.Background(), work()); err != nil {
		t.Fatal(err)
	}
	requireOrder(t, calls, "route:new", "check:new", "route:old", "check:old", "remove:new", "report:failed")
	forbidden(t, calls, "stop:old", "report:active")
}
func TestLostActivationAcknowledgementRetainsBothContainers(t *testing.T) {
	calls := []string{}
	r, p := setup(&calls)
	p.activeErr = errors.New("database unavailable")
	if err := r.Run(context.Background(), work()); err == nil {
		t.Fatal("expected retry")
	}
	forbidden(t, calls, "stop:old", "remove:new", "route:old", "report:failed")
}
func TestFailedRouteRestorationDoesNotDestroyCandidate(t *testing.T) {
	calls := []string{}
	r, _ := setup(&calls)
	r.Runtime = fakeRuntime{calls: &calls, pullErr: errors.New("bad image")}
	r.Routes = fakeRoutes{calls: &calls, restoreErr: errors.New("disk full")}
	if err := r.Run(context.Background(), work()); err == nil {
		t.Fatal("expected retry")
	}
	forbidden(t, calls, "remove:new", "report:failed", "stop:old")
}
func TestShutdownLeavesJobResumable(t *testing.T) {
	calls := []string{}
	r, _ := setup(&calls)
	ctx, cancel := context.WithCancel(context.Background())
	r.Check = func(context.Context, string, string, string) error { cancel(); return ctx.Err() }
	if err := r.Run(ctx, work()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	forbidden(t, calls, "report:failed", "remove:new", "stop:old")
}
func TestFirstDeploymentFailureRemovesRoute(t *testing.T) {
	calls := []string{}
	r, _ := setup(&calls)
	r.Runtime = fakeRuntime{calls: &calls, pullErr: errors.New("bad image")}
	w := work()
	w.Previous = nil
	if err := r.Run(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	requireOrder(t, calls, "route:none", "remove:new", "report:failed")
}
