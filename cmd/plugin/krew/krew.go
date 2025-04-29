package krew

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	// https://krew.sigs.k8s.io/docs/developer-guide/develop/best-practices/
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/prequel-dev/preq/internal/pkg/cli"
	"github.com/prequel-dev/preq/internal/pkg/logs"
	"github.com/prequel-dev/preq/internal/pkg/ux"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	ErrInvalidResource = errors.New("invalid resource")
)

var (
	KubernetesConfigFlags *genericclioptions.ConfigFlags
	k8sDeployment         = "deployment"
	k8sJob                = "job"
	k8sService            = "service"
	k8sPod                = "pod"
)

type krewOptions struct {
	genericclioptions.IOStreams
	flags        *genericclioptions.ConfigFlags
	namespace    string
	resource     string
	clientConfig *rest.Config
}

func NewRunOptions(streams genericclioptions.IOStreams) *krewOptions {
	return &krewOptions{
		IOStreams: streams,
		flags:     genericclioptions.NewConfigFlags(true),
	}
}

func InitAndExecute(ctx context.Context, streams genericclioptions.IOStreams) {
	o := NewRunOptions(streams)

	if err := RootCmd(ctx, o).Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func (o *krewOptions) getCmdFactory(cmd *cobra.Command) cmdutil.Factory {
	flags := cmd.PersistentFlags()
	o.flags.AddFlags(flags)

	matchVersionFlags := cmdutil.NewMatchVersionFlags(o.flags)
	matchVersionFlags.AddFlags(flags)

	return cmdutil.NewFactory(matchVersionFlags)
}

func (o *krewOptions) getNamespace(factory cmdutil.Factory) error {
	var err error
	if o.namespace, _, err = factory.ToRawKubeConfigLoader().Namespace(); err != nil {
		return err
	}
	return nil
}

func (o *krewOptions) getClientConfig(factory cmdutil.Factory) error {
	var err error
	if o.clientConfig, err = factory.ToRESTConfig(); err != nil {
		return err
	}
	return nil
}

func RootCmd(ctx context.Context, o *krewOptions) *cobra.Command {

	cmd := &cobra.Command{
		Use:           ux.KrewUsage,
		Short:         ux.KrewDescShort,
		Long:          ux.KrewDescLong,
		Example:       ux.KrewExamples,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	factory := o.getCmdFactory(cmd)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			o.resource = args[0]
		}

		if err := o.getNamespace(factory); err != nil {
			return err
		}

		if err := o.getClientConfig(factory); err != nil {
			return err
		}

		return runPreq(ctx, o)
	}

	// preq options
	cmd.Flags().BoolVarP(&cli.Options.Disabled, "disabled", "d", false, ux.HelpDisabled)
	cmd.Flags().BoolVarP(&cli.Options.Cron, "cronjob", "j", false, ux.HelpCron)
	cmd.Flags().StringVarP(&cli.Options.Level, "level", "l", "", ux.HelpLevel)
	cmd.Flags().StringVarP(&cli.Options.Name, "name", "o", "", ux.HelpName)
	cmd.Flags().BoolVarP(&cli.Options.Quiet, "quiet", "q", false, ux.HelpQuiet)
	cmd.Flags().StringVarP(&cli.Options.Rules, "rules", "r", "", ux.HelpRules)
	cmd.Flags().BoolVarP(&cli.Options.Version, "version", "v", false, ux.HelpVersion)
	cmd.Flags().BoolVarP(&cli.Options.AcceptUpdates, "accept-updates", "y", false, ux.HelpAcceptUpdates)

	cobra.OnInitialize(initConfig)

	return cmd
}

func initConfig() {
	viper.AutomaticEnv()
}

func podsForSelector(ctx context.Context, cs *kubernetes.Clientset,
	namespace string, sel map[string]string) ([]v1.Pod, error) {

	labelSel := labels.SelectorFromSet(sel).String()

	podList, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSel,
	})
	if err != nil {
		log.Error().Err(err).Msg("podsForSelector")
		return nil, err
	}

	log.Info().Any("podList", podList.Items).Msg("podsForSelector")

	return podList.Items, nil
}

func podsForDeployment(ctx context.Context, cs *kubernetes.Clientset,
	namespace, name string) ([]v1.Pod, error) {

	dep, err := cs.AppsV1().Deployments(namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return podsForSelector(ctx, cs, namespace,
		dep.Spec.Selector.MatchLabels)
}

func podsForJob(ctx context.Context, cs *kubernetes.Clientset,
	namespace, name string) ([]v1.Pod, error) {

	job, err := cs.BatchV1().Jobs(namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		log.Error().Err(err).Msg("podsForJob")
		return nil, err
	}

	return podsForSelector(ctx, cs, namespace,
		job.Spec.Selector.MatchLabels)
}

func podsForService(ctx context.Context, cs *kubernetes.Clientset,
	namespace, svcName string) ([]v1.Pod, error) {

	svc, err := cs.CoreV1().Services(namespace).
		Get(ctx, svcName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return podsForSelector(ctx, cs, namespace, svc.Spec.Selector)
}

type resourceT struct {
	name string
	kind string
}

func getResource(r string) (resourceT, error) {
	if strings.Contains(r, "/") {
		parts := strings.Split(r, "/")

		if len(parts) != 2 {
			return resourceT{}, fmt.Errorf("invalid resource: %s", r)
		}

		resource := resourceT{
			name: parts[1],
			kind: parts[0],
		}

		log.Info().
			Str("name", resource.name).
			Str("kind", resource.kind).
			Msg("getResource")

		return resource, nil
	}

	// Assume pod by default
	return resourceT{
		name: r,
		kind: k8sPod,
	}, nil
}

func runPreq(ctx context.Context, o *krewOptions) error {

	logOpts := []logs.InitOpt{
		logs.WithLevel(cli.Options.Level),
	}

	logs.InitLogger(logOpts...)

	log.Info().Msgf("resource: %s", o.resource)

	if o.resource != "" {

		var (
			resource resourceT
			err      error
		)

		if resource, err = getResource(o.resource); err != nil {
			log.Error().Err(err).Str("resource", o.resource).Msg("invalid resource")
			return err
		}

		clientset, err := kubernetes.NewForConfig(o.clientConfig)
		if err != nil {
			return err
		}

		switch resource.kind {
		case k8sPod:
			return redirectPodLogs(ctx, clientset, o.namespace, resource.name)
		case k8sDeployment:
			pods, err := podsForDeployment(ctx, clientset, o.namespace, resource.name)
			if err != nil {
				return err
			}

			for _, pod := range pods {
				if err := redirectPodLogs(ctx, clientset, o.namespace, pod.Name); err != nil {
					return err
				}
			}
		case k8sJob:
			pods, err := podsForJob(ctx, clientset, o.namespace, resource.name)
			if err != nil {
				return err
			}

			for _, pod := range pods {
				if err := redirectPodLogs(ctx, clientset, o.namespace, pod.Name); err != nil {
					return err
				}
			}
		case k8sService:
			pods, err := podsForService(ctx, clientset, o.namespace, resource.name)
			if err != nil {
				return err
			}

			for _, pod := range pods {
				if err := redirectPodLogs(ctx, clientset, o.namespace, pod.Name); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func redirectPodLogs(ctx context.Context, clientset *kubernetes.Clientset, namespace, pod string) error {

	var (
		rdr io.ReadCloser
		err error
	)

	rdr, err = clientset.CoreV1().Pods(namespace).GetLogs(pod, &v1.PodLogOptions{}).Stream(ctx)
	if err != nil {
		return err
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}

	go func() {
		defer pw.Close()
		if _, err := io.Copy(pw, rdr); err != nil {
			log.Warn().Err(err).Msg("copy logs -> pipe failed")
		}
	}()

	os.Stdin = pr

	return cli.InitAndExecute(ctx)
}
