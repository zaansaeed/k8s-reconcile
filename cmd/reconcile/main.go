package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"k8s.io/client-go/tools/clientcmd"
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("finding home dir: %v", err)
	}
	
	//name, default, usage
	kubeconfig := flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "path to kubeconfig")
	namespace := flag.String("namespace", "default", "namespace to list")
	// parse arguments passed into terminal
	flag.Parse()

	//config of kubernetes cluster(s)
	cfg, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)

	if err != nil {
		log.Fatalf("loading kubeconfig: %v", err)
	}

	


	clientset, err := kubernetes.NewForConfig(cfg) //create clientset based on clusterconfig
	if err != nil {
		log.Fatalf("creating clientset: %v", err)
	}

	// any req that takes longer than 10 sec, kill
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	//nodes
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Fatalf("listing nodes: %v", err)
	}
	for _, n := range nodes.Items {
		fmt.Println("node:", n.Name)
	}

	//pods
	pods, err := clientset.CoreV1().Pods(*namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Fatalf("listing pods: %v", err)
	}
	for _, p := range pods.Items {
		fmt.Println("pod:", p.Name, p.Status.Phase)
	}
}