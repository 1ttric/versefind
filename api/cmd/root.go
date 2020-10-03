package cmd

import (
	"flag"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"versefind/pkg"
)

func init() {
	rootCmd.PersistentFlags().StringVar(&listenAddr, "listen", "0.0.0.0:3001", "the address and port on which to listen")
	rootCmd.PersistentFlags().StringVar(&oauthRedirectAddr, "oauthredirectaddr", "https://versefind.com/callback", "the oauth redirect endpoint")
	rootCmd.PersistentFlags().StringVar(&esAddr, "elastic", "http://127.0.0.1:9200", "the Elastic instance in which to cache track data and lyric content")
	flag.Parse()
}

var (
	verbosity         string
	listenAddr        string
	oauthRedirectAddr string
	esAddr            string

	rootCmd = &cobra.Command{
		Use:   "hddcheap",
		Short: "The backend for the hddcheap application",
		Long:  "hddcheap is a single page webapp for quickly finding cheap spinning rust on Amazon",
		RunE: func(cmd *cobra.Command, args []string) error {
			level, err := log.ParseLevel(verbosity)
			if err != nil {
				return err
			}
			log.SetLevel(level)
			log.SetReportCaller(true)
			pkg.Serve(listenAddr, oauthRedirectAddr, esAddr)
			return nil
		},
	}
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatalf(err.Error())
	}
}
