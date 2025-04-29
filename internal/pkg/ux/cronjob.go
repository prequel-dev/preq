package ux

import (
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
)

var (
	JobTemplate = `# ---------------------------------------------------------------------------
# preq cronjob template
# 
# PRE-RUN Create/refresh the ConfigMap that the CronJob expects:
#
# Option 1: Use default latest rules with a Slack notification webhook
# 
#   kubectl create configmap preq-conf \
#     --from-file=config.yaml=%s/config.yaml \
#     --from-file=.ruletoken=%s/.ruletoken \
#     --from-file=%s=%s/%s \
#     --dry-run=client -o yaml | kubectl apply -f -
#
# The --dry-run/apply pattern lets you update the ConfigMap idempotently.
# 
# These configuration files are automatically created by preq the first time it is executed locally by the kubectl client. 
# 
# NOTE: This template assumes the config.yaml file is configured to use a Slack notification webhook. Visit 
# https://docs.prequel.dev/configuration to learn how to modify the configuration file to add a notification webhook (e.g. Slack).
#
# notification:
#   type: slack
#   webhook: https://hooks.slack.com/services/.....
#
# Option 2: Use custom rules with a Slack notification webhook
#
# To add custom rules to this job, update the config.yaml file to add the path to your custom rules file where it will be mounted 
# in the cronjob filesystem.
#
# rules:
#   paths:
#     - /.preq/custom-rules.yaml
#
# Then create the configmap with the following command:
#
#   kubectl create configmap preq-conf \
#     --from-file=config.yaml=%s/config.yaml \
#     --from-file=.ruletoken=%s/.ruletoken \
#     --from-file=%s=%s/%s \
#     --from-file=custom-rules.yaml=/local/path/to/custom-rules.yaml \
#     --dry-run=client -o yaml | kubectl apply -f -
#
# IMPORTANT:
# 
# 1. Uncomment the command in the job below to add a POD to monitor. Use labels to select the POD for a service.
# 2. Update the schedule to run at the frequency you want. This runs every 10 minutes by default.
# 3. Change the -o "preq-cronjob-<POD>: " output prefix to the name of the cronjob or how you want to identify these notifications in Slack.
#
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
    resources: ['pods', 'pods/log']
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
                  # kubectl preq "$POD" -y -o "preq-cronjob-<POD>: "
				  #
				  # * If you want to monitor pods in a deployment, use the following command:
				  # kubectl preq deployment/<DEPLOYMENT> -y -o "preq-cronjob-<DEPLOYMENT>: "
				  #
				  # * If you want to monitor pods in a job, use the following command:
				  # kubectl preq job/<JOB> -y -o "preq-cronjob-<JOB>: "
				  #
				  # * If you want to monitor pods in a service, use the following command:
				  # kubectl preq service/<SERVICE> -y -o "preq-cronjob-<SERVICE>: "
				  
              volumeMounts:
                - name: preq-conf
                  mountPath: /.preq
                  readOnly: true
          restartPolicy: Never
          volumes:
            - name: preq-conf
              configMap:
                name: preq-conf
          serviceAccountName: preq
`
	ConfigMapStdoutTemplate = `
kubectl create configmap preq-conf \
  --from-file=config.yaml=%s/config.yaml \
  --from-file=.ruletoken=%s/.ruletoken \
  --from-file=%s=%s/%s
`
)

func PrintCronJobTemplate(output, configDir, rulesFile string) error {
	if output == OutputStdout {
		fmt.Fprintf(os.Stdout, JobTemplate, configDir, configDir, rulesFile, configDir, rulesFile, configDir, configDir, rulesFile, configDir, rulesFile)
	} else {

		if output == "" {
			output = "cronjob.yaml"
		}

		job := fmt.Sprintf(JobTemplate, configDir, configDir, rulesFile, configDir, rulesFile, configDir, configDir, rulesFile, configDir, rulesFile)
		err := os.WriteFile(output, []byte(job), 0644)
		if err != nil {
			log.Error().Err(err).Msg("Failed to write cronjob template")
			return err
		}

		fmt.Fprintln(os.Stdout, "Cronjob template written to", output)
	}

	return nil
}
