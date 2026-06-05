package helpers

import "fmt"

// Report tracks pass/fail counts for a test block.
type Report struct {
	Pass  int
	Fail  int
	Total int
	p     Printer
}

// NewReport returns a Report backed by the default terminal printer.
func NewReport() *Report {
	return &Report{p: DefaultPrinter}
}

func (r *Report) Passf(format string, args ...interface{}) {
	r.Pass++
	r.Total++
	r.p.Pass(fmt.Sprintf(format, args...))
}

func (r *Report) PassMsg(msg string) { r.Passf("%s", msg) }

func (r *Report) Failf(format string, args ...interface{}) {
	r.Fail++
	r.Total++
	r.p.Fail(fmt.Sprintf(format, args...))
}

func (r *Report) FailMsg(msg string) { r.Failf("%s", msg) }

func (r *Report) Section(title string) { r.p.Section(title) }

func (r *Report) PrintResults(pointName string) {
	fmt.Printf("\n%s%s━━━ Results: %s ━━━%s\n\n", AnsiBold, AnsiNC, pointName, AnsiNC)
	fmt.Printf("  %sPassed:%s %d ", AnsiGreen, AnsiNC, r.Pass)
	fmt.Printf("  %sFailed:%s %d ", AnsiRed, AnsiNC, r.Fail)
	fmt.Printf("  Total: %d\n", r.Total)
}

// Merge adds src counts into r.
func (r *Report) Merge(src *Report) {
	r.Pass += src.Pass
	r.Fail += src.Fail
	r.Total += src.Total
}
