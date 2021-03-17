# Kubernetes data replicator

This project provides a way to replicate data from a parent New Relic account to subaccounts. Data is retrieved using a NRQL query from the parent account by NRQL and pushed as Metrics to the subaccounts. Events replication is not supported.

## How it works

1) The pod is launched through CronJob and checks for the right access credentials to the Kubernetes API and New Relic API.
2) The pod reads in the configuration and queries to execute
3) The pod scans all namespaces and looks for a predefined secret containing the subaccount information of where to push the information.
4) For each namespace with a valid secret the queries are executed and the data is pushed.

## Set-up

### Set the parent account secrets in the namespace you want to run the CronJob

Replace the `{{PARENT_ACCOUNT_ID}}` with the ID of your New Relic master account, `{{PARENT_USER_TOKEN}}` with a [**User API key**](https://docs.newrelic.com/docs/apis/get-started/intro-apis/new-relic-api-keys/) and `{{NAMESPACE}}` where New Relic K8s integration is running.

`kubectl create secret generic nr-replicator-parent-secret -n {{NAMESPACE}} --from-literal parentAccountId='{{PARENT_ACCOUNT_ID}}' --from-literal parentUserToken='{{PARENT_USER_TOKEN}}'`

### For each namespace you want to replicate data for, create the following secret

Replace the `{{CHILD_ACCOUNT_ID}}` with the ID of the New Relic account where you want to replicate the data, `{{CHILD_INSIGHTS_TOKEN}}` with a [**Insights insert key**](https://docs.newrelic.com/docs/apis/get-started/intro-apis/new-relic-api-keys/) for that account and `{{CHILD_NAMESPACE}}` with namespace you want to replicate data for.

`kubectl create secret generic nr-replicator-secret -n {{CHILD_NAMESPACE}} --from-literal=accountId='{{CHILD_ACCOUNT_ID}}' --from-literal=apiToken='{{CHILD_INSIGHTS_TOKEN}}`

### Change the manifest.yml file to your liking and deploy

`manifest.yml` contains the queries that will be run to replicate data from master to child accounts. Facets will be added as dimensions to the metric data, including the current namespace. The metrics will show up in New Relic as `k8s-replicator.[name]`. Make sure you have no overlapping `[name]` fields, use `value as [name]` to set it in the NRQL query.

For example the query `SELECT average(cpuUsedCores) as 'cpu-cores-utilization', average(memoryUsedBytes) as 'memory-usage' FROM K8sContainerSample FACET podName, containerName WHERE namespace = '$namespace' SINCE 60 minutes ago LIMIT 1000` will create the `k8s-replicator.cpu-cores-utilization` and `k8s-replicator.memory-usage` metrics with following dimensions: podName, containerName, namespace.

`manifest.yml` also contains the `schedule` of the replication, by default this is every minute `* * * * *`, but you can change this to every hour if needed `0 * * * *`. The default will create metrics every minute, so keep in mind that this will increase your New Relic data usage and bill. The increase depends on the number of queries in the schedule.

Once you're happy with the config, deploy with the following command, don't forget to replace `{{NAMESPACE}}` with the namespace where New Relic K8s integration is running.

`kubectl apply -n {{NAMESPACE}} -f manifest.yml`

## Development

Running locally: `cd src/` + `POD_NAMESPACE=newrelic go run .`
