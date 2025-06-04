package ux

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

var (
	JobTemplate = `# ---------------------------------------------------------------------------
# preq cronjob template
# 
# Step 1. Create/refresh the ConfigMap that the CronJob expects:
#
#   kubectl create configmap preq-conf \
#     --from-file=config.yaml=%s/config.yaml \
#     --from-file=.ruletoken=%s/.ruletoken \
#     --from-file=%s=%s/%s \
#     --dry-run=client -o yaml | kubectl apply -f -
#
# The --dry-run/apply pattern lets you update the ConfigMap idempotently.
#
# Step 2. Install the job
#
#   kubectl apply -f cronjob.yaml
#
# IMPORTANT:
# 
# 1. Uncomment the command in the job below to add a deployment, pod, job, or service to monitor. Use labels to select the POD for a service.
# 2. Update the schedule to run at the frequency you want. This runs every 10 minutes by default.
# 3. Change the actions.yaml to run an executable or create a JIRA ticket instead of sending a Slack notification.
#
# Visit https://docs.prequel.dev for more information.
# ---------------------------------------------------------------------------
apiVersion: v1
kind: ServiceAccount
metadata:
  name: preq
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: preq
rules:
  - apiGroups: ['']
    resources: ['services', 'jobs', 'depoyments', 'pods', 'pods/log']
    verbs: ['get', 'list', 'watch']
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: preq
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: preq
subjects:
  - kind: ServiceAccount
    name: preq
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: preq-cronjob
spec:
  schedule: "*/10 * * * *"       # every 10 minutes
  concurrencyPolicy: Forbid   # don’t start a new run until the prior run finishes
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 3
  jobTemplate:
    spec:
      backoffLimit: 1
      template:
        spec:
          containers:
            - name: preq-cronjob
              image: prequeldev/kubectl-krew-preq:latest
              command:
                - /bin/sh
                - -c
                - |
                  ############
                  # IMPORTANT: Uncomment the command in the job below
                  #
                  # * If you want to monitor a pod using labels to select the POD for a service, use the following commands:
                  # POD=$(kubectl -n default get pods -l app.kubernetes.io/instance=<LABEL> -o jsonpath='{.items[0].metadata.name}')
                  # kubectl preq "$POD" -y
                  #
                  # * If you want to monitor pods in a deployment, use the following command:
                  # kubectl preq deployment/<DEPLOYMENT> -y
                  #
                  # * If you want to monitor pods in a job, use the following command:
                  # kubectl preq job/<JOB> -y
                  #
                  # * If you want to monitor pods in a service, use the following command:
                  # kubectl preq service/<SERVICE> -y 
				  
              volumeMounts:
                - name: preq-conf
                  mountPath: /.config/preq
                  readOnly: true
                - name: actions-config
                  mountPath: /.config/preq/actions.yaml
                  readOnly: true
          restartPolicy: Never
          volumes:
            - name: preq-conf
              configMap:
                name: preq-conf
            - name: actions-config
              configMap:
                name: actions-config
          serviceAccountName: preq
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: actions-config
data:
  actions.yaml: |-
    actions:
      - type: slack
        regex: "CRE*"
        slack:
          webhook_url: <SLACK_WEBHOOK_URL>
          message_template: |
            *preq detection*: [{{ field .cre "Id" }}] {{ field .cre "Title" }}

            {{ (index .hits 0).Timestamp }}: {{ (index .hits 0).Entry }}
`
	ConfigMapStdoutTemplate = `
kubectl create configmap preq-conf \
  --from-file=config.yaml=%s/config.yaml \
  --from-file=.ruletoken=%s/.ruletoken \
  --from-file=%s=%s/%s
`
)

func PrintCronJobTemplate(output, configDir, rulesPath string) error {

	rulesFile := filepath.Base(rulesPath)

	if output == OutputStdout {
		fmt.Fprintf(os.Stdout, JobTemplate, configDir, configDir, rulesFile, configDir, rulesFile)
	} else {

		if output == "" {
			output = "cronjob.yaml"
		}

		job := fmt.Sprintf(JobTemplate, configDir, configDir, rulesFile, configDir, rulesFile)
		err := os.WriteFile(output, []byte(job), 0644)
		if err != nil {
			log.Error().Err(err).Msg("Failed to write cronjob template")
			return err
		}

		fmt.Fprintln(os.Stdout, "Cronjob template written to", output)
	}

	return nil
}
