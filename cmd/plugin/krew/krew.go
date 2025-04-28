package krew

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	// https://krew.sigs.k8s.io/docs/developer-guide/develop/best-practices/
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/prequel-dev/preq/internal/pkg/cli"
	"github.com/prequel-dev/preq/internal/pkg/logs"
	"github.com/prequel-dev/preq/internal/pkg/timez"
	"github.com/prequel-dev/preq/internal/pkg/ux"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	ErrPodRequired = errors.New("POD is required")
)

var (
	KubernetesConfigFlags *genericclioptions.ConfigFlags
)

type krewOptions struct {
	genericclioptions.IOStreams
	flags        *genericclioptions.ConfigFlags
	namespace    string
	pod          string
	container    string
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
		if len(args) != 1 {
			return ErrPodRequired
		}

		o.pod = args[0]

		if err := o.getNamespace(factory); err != nil {
			return err
		}

		if err := o.getClientConfig(factory); err != nil {
			return err
		}

		return runPreq(ctx, o)
	}

	// kubectl-specific option
	cmd.Flags().StringVarP(&o.container, "container", "c", o.container, "Specify the container name")

	// preq options
	cmd.Flags().BoolVarP(&cli.Options.SlackNotification, "slack", "a", false, "Send a Slack notification to the configured webhook when one or more CRE is detected")
	cmd.Flags().BoolVarP(&cli.Options.Disabled, "disabled", "d", false, "Do not run community CREs")
	cmd.Flags().StringVarP(&cli.Options.Stop, "stop", "e", "", "Stop time")
	cmd.Flags().BoolVarP(&cli.Options.JsonLogs, "json", "j", false, "Print logs in JSON format to stderr")
	cmd.Flags().IntVarP(&cli.Options.Skip, "skip", "k", timez.DefaultSkip, "Skip the first N lines for timestamp detection")
	cmd.Flags().StringVarP(&cli.Options.Level, "level", "l", "", "Print logs at this level to stderr")
	cmd.Flags().StringVarP(&cli.Options.Name, "name", "o", "", "Report output name, generated data source template name, or notification context name")
	cmd.Flags().BoolVarP(&cli.Options.Quiet, "quiet", "q", false, "Quiet mode, do not print progress")
	cmd.Flags().StringVarP(&cli.Options.Rules, "rules", "r", "", "Path to a CRE rules file")
	cmd.Flags().StringVarP(&cli.Options.Format, "format", "t", "", "Format to use for timestamps")
	cmd.Flags().BoolVarP(&cli.Options.Version, "version", "v", false, "Print version and exit")
	cmd.Flags().StringVarP(&cli.Options.Window, "window", "w", "", "Reorder lookback window duration")
	cmd.Flags().StringVarP(&cli.Options.Regex, "regex", "x", "", "Regex to match for extracting timestamps")
	cmd.Flags().BoolVarP(&cli.Options.AcceptUpdates, "accept-updates", "y", false, "Accept updates to rules or new release")

	cobra.OnInitialize(initConfig)

	return cmd
}

func initConfig() {
	viper.AutomaticEnv()
}

func runPreq(ctx context.Context, o *krewOptions) error {

	var err error

	clientset, err := kubernetes.NewForConfig(o.clientConfig)
	if err != nil {
		return err
	}

	src, err := clientset.CoreV1().Pods(o.namespace).GetLogs(o.pod, &v1.PodLogOptions{
		Container: o.container,
	}).Stream(context.Background())
	if err != nil {
		return err
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}

	go func() {
		defer pw.Close()
		if _, err := io.Copy(pw, src); err != nil {
			log.Warn().Err(err).Msg("copy logs -> pipe failed")
		}
	}()

	oldStdin := os.Stdin
	os.Stdin = pr
	defer oldStdin.Close()

	logOpts := []logs.InitOpt{
		logs.WithLevel(cli.Options.Level),
	}

	logs.InitLogger(logOpts...)

	cli.InitAndExecute(ctx)

	return nil
}
