package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Unable to find home dir: %v", err)
	}
	
	//name, default, useage
	kubeconfig := flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "path to kubeconfig")

	// parse arguments passed into terminal
	flag.Parse()

	cfg, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)

	if err != nil {
		log.Fatalf("loading kubeconfig: %v", err)
	}

	fmt.Println("API server:", cfg.Host)
}