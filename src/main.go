/*
Copyright 2016 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Note: the example only works with the code within the same release/branch.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/machinebox/graphql"
	"github.com/newrelic/newrelic-telemetry-sdk-go/telemetry"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// configuration set in config map
var replicatorConfiguration struct {
	Parent struct {
		ApiToken  string `yaml:"apiToken"`
		AccountId int    `yaml:"accountId"`
	} `yaml:"parent"`
	Queries []string `yaml:"queries,flow"`
}

// clientset used to communicate with k8s
var clientset *kubernetes.Clientset

// graphQl client to communicate with New Relic
var graphQLClient *graphql.Client

func main() {
	log.Println("Starting data replication")
	log.Println("Connecting to K8s API")

	// find local kubeconfig file, this is used for local development
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		// falling back to dev
		log.Printf("Tried connecting using rest API, but failed, retrying with local config. Error message: %s\n", err.Error())
		// use the current context in kubeconfig
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			log.Fatal("Can't read current context", err.Error())
		} else {
			log.Println("K8s current context created")
		}
	}

	// create the clientset
	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal("Can't create clientset", err.Error())
	} else {
		log.Println("K8s clientset created")
	}

	// create a client (safe to share across requests)
	graphQLClient = graphql.NewClient("https://api.newrelic.com/graphql")

	// get configuration in my namespace and parse
	configuration, err := clientset.CoreV1().ConfigMaps("default").Get(context.TODO(), "nr-replicator-config", metav1.GetOptions{})
	if err != nil {
		log.Fatal("Unable to read configuration, please check if nr-replicator-config exists", err.Error())
		panic(err.Error())
	}

	// parse configuration  data
	err = yaml.UnmarshalStrict([]byte(configuration.Data["config"]), &replicatorConfiguration)
	if err != nil {
		log.Fatal("Failed to parse file ", err)
	} else {
		log.Printf("Finished reading configuration, starting replication for account %d\n", replicatorConfiguration.Parent.AccountId)
	}

	// retrieve all namespaces
	namespaces, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatal("Unable to read all namespaces, please check permissions", err.Error())
		panic(err.Error())
	} else {
		log.Println("List of namespaces received, starting processing:")
	}

	// check for account secret, and token
	for _, namespace := range namespaces.Items {
		namespaceName := namespace.Name
		log.Printf("namespace '%s' - Checking namespace for secret", namespaceName)

		processNamespace(namespaceName)
	}
}

func processNamespace(namespace string) {
	secret, err := clientset.CoreV1().Secrets(namespace).Get(context.TODO(), "nr-replicator-secret", metav1.GetOptions{})
	if err != nil {
		log.Printf("namespace '%s' - Secret not found, error received: '%s'\n", namespace, err.Error())
		log.Printf("namespace '%s' - We will not continue with this namespace, please create the secret if you want to process this namespace.", namespace)
		return
	}
	accountId, _ := strconv.Atoi(string(secret.Data["accountId"]))
	apiToken := string(secret.Data["apiToken"])
	log.Printf("namespace '%s' - Found secret, with New Relic accountId: %d\n", namespace, accountId)

	for _, query := range replicatorConfiguration.Queries {
		query = strings.Replace(query, "$namespace", namespace, -1)
		log.Printf("namespace '%s' - Running query: \n%s\n", namespace, query)
		metrics := getMetrics(accountId, apiToken, query)

		// First create a Harvester.  APIKey is the only required field.
		h, err := telemetry.NewHarvester(
			telemetry.ConfigAPIKey(apiToken),
			telemetry.ConfigBasicErrorLogger(log.Writer()),
		)
		if err != nil {
			fmt.Println(err)
		}

		// Record all metrics
		for _, metric := range metrics {
			h.RecordMetric(metric)
			log.Printf("Sending metric: %s\n", metric.Name)
		}

		// By default, the Harvester sends metrics and spans to the New Relic
		// backend every 5 seconds.  You can force data to be sent at any time
		// using HarvestNow.
		h.HarvestNow(context.TODO())
		log.Printf("Finished namespace %s\n", namespace)
	}
}

func getMetrics(accountId int, apiToken string, query string) []telemetry.Gauge {
	// make a request
	req := graphql.NewRequest(`
		query ($accountId: Int!, $query: Nrql!){
			actor {
				account(id: $accountId) {
					nrql(query: $query) {
						totalResult
						results
						metadata {
							facets
						}
					}
				}
			}
		}
	`)
	req.Var("accountId", replicatorConfiguration.Parent.AccountId)
	req.Var("query", query)
	req.Header.Set("API-Key", replicatorConfiguration.Parent.ApiToken)

	// define a Context for the request
	ctx := context.TODO()
	var respData struct {
		Actor struct {
			Account struct {
				Nrql struct {
					Metadata struct {
						Facets []string
					}
					Results     []map[string]interface{}
					TotalResult map[string]float64
				}
			}
		}
	}

	// get data
	if err := graphQLClient.Run(ctx, req, &respData); err != nil {
		log.Fatal(err)
	}

	// define metrics return
	var metrics []telemetry.Gauge

	// loop results and prep metrics
	for name, value := range respData.Actor.Account.Nrql.TotalResult {
		// prep attributes
		attributes := map[string]interface{}{}
		for _, attribute := range respData.Actor.Account.Nrql.Metadata.Facets {
			attributes[attribute] = respData.Actor.Account.Nrql.Results[0][attribute]
		}

		metrics = append(metrics, telemetry.Gauge{
			Timestamp:  time.Now(),
			Value:      value,
			Name:       "k8s-replicator." + name,
			Attributes: attributes,
		})
	}

	return metrics
}
