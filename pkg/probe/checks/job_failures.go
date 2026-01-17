package checks

import (
	"context"
	"fmt"
	"time"

	"github.com/punasusi/cluster-probe/pkg/probe"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type JobFailures struct{}

func NewJobFailures() *JobFailures {
	return &JobFailures{}
}

func (c *JobFailures) Name() string {
	return "job-failures"
}

func (c *JobFailures) Tier() int {
	return 2
}

func (c *JobFailures) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:		c.Name(),
		Tier:		c.Tier(),
		Results:	[]probe.Result{},
	}

	jobs, err := client.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}

	activeJobs := 0
	succeededJobs := 0
	failedJobs := 0

	for _, job := range jobs.Items {

		if job.Status.Succeeded > 0 && job.Status.Failed == 0 {
			succeededJobs++
			continue
		}

		if job.Status.Active > 0 {
			activeJobs++

			if job.Status.StartTime != nil {
				runTime := time.Since(job.Status.StartTime.Time)
				if runTime > 1*time.Hour {
					result.Results = append(result.Results, probe.Result{
						CheckName:	c.Name(),
						Severity:	probe.SeverityWarning,
						Message:	fmt.Sprintf("Job %s/%s has been running for %s", job.Namespace, job.Name, formatDuration(runTime)),
						Details: []string{
							fmt.Sprintf("Active pods: %d", job.Status.Active),
						},
						Remediation:	"Check job pod logs for issues",
					})
				}
			}
		}

		if job.Status.Failed > 0 {
			failedJobs++

			var failureReason string
			for _, cond := range job.Status.Conditions {
				if cond.Type == batchv1.JobFailed {
					failureReason = cond.Message
				}
			}

			isRecent := false
			if job.Status.CompletionTime != nil {
				if time.Since(job.Status.CompletionTime.Time) < 24*time.Hour {
					isRecent = true
				}
			} else if job.Status.StartTime != nil {
				if time.Since(job.Status.StartTime.Time) < 24*time.Hour {
					isRecent = true
				}
			}

			if isRecent {
				details := []string{
					fmt.Sprintf("Failed: %d", job.Status.Failed),
				}
				if failureReason != "" {
					details = append(details, fmt.Sprintf("Reason: %s", failureReason))
				}

				result.Results = append(result.Results, probe.Result{
					CheckName:	c.Name(),
					Severity:	probe.SeverityWarning,
					Message:	fmt.Sprintf("Job %s/%s has failed", job.Namespace, job.Name),
					Details:	details,
					Remediation:	fmt.Sprintf("Check job pods: kubectl get pods -n %s -l job-name=%s", job.Namespace, job.Name),
				})
			}
		}
	}

	cronJobs, err := client.BatchV1().CronJobs("").List(ctx, metav1.ListOptions{})
	if err == nil {
		suspendedCronJobs := 0
		for _, cj := range cronJobs.Items {
			if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
				suspendedCronJobs++
			}

			if cj.Status.LastScheduleTime != nil {
				lastSchedule := cj.Status.LastScheduleTime.Time
				age := time.Since(lastSchedule)

				if age > 2*time.Hour && (cj.Spec.Suspend == nil || !*cj.Spec.Suspend) {
					result.Results = append(result.Results, probe.Result{
						CheckName:	c.Name(),
						Severity:	probe.SeverityWarning,
						Message:	fmt.Sprintf("CronJob %s/%s last ran %s ago", cj.Namespace, cj.Name, formatDuration(age)),
						Details: []string{
							fmt.Sprintf("Schedule: %s", cj.Spec.Schedule),
							fmt.Sprintf("Active jobs: %d", len(cj.Status.Active)),
						},
						Remediation:	"Verify cron schedule and check controller-manager logs",
					})
				}
			}
		}
	}

	severity := probe.SeverityOK
	if failedJobs > 0 {
		severity = probe.SeverityWarning
	}

	result.Results = append(result.Results, probe.Result{
		CheckName:	c.Name(),
		Severity:	severity,
		Message:	fmt.Sprintf("Jobs: %d active, %d succeeded, %d failed", activeJobs, succeededJobs, failedJobs),
		Details: []string{
			fmt.Sprintf("Total jobs: %d", len(jobs.Items)),
		},
	})

	return result, nil
}
