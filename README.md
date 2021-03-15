# Kubernetes data replicator

This project provides a way to replicate data from a parent New Relic account to subaccounts. Data is retrieved using a NRQL query from the parent account by NRQL and pushed as Metrics to the subaccounts. Events replication is not supported.

## How it works

1) The pod is launched through CronJob and checks for the right access credentials to the Kubernetes API and New Relic API.
2) The pod reads in the configuration and queries to execute
3) The pod scans all namespaces and looks for a predefined secret containing the subaccount information of where to push the information.
4) For each namespace with a valid secret the queries are executed and the data is pushed.


