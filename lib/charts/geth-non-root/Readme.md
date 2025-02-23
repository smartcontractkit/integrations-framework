# Raison d'etre

This chart allows to run a geth node as a non-root user, which is essential for running it on more secure clusters. Geth is running as Proof-of-Authority private network with a single node. By default ingress is disabled, so remember to enable it in `values.yaml`.

Sample command:
```bash
export RELEASE_NAME="your-release-name"
export NAMESPACE="your-namespace"
export INGRESS_BASE_DOMAIN="your-ingress-base-domain"
export INGRESS_CERT_ARN="your-ingress-certificate"
export INGRESS_CIDRS="allowed-cidrs"

helm install "${RELEASE_NAME}" . -f ./values.yaml \
--set ingress.annotation_certificate_arn="${INGRESS_CERT_ARN}" \
--set "ingress.hosts[0].host"="${NAMESPACE}-geth-http.${INGRESS_BASE_DOMAIN}" \
--set "ingress.hosts[1].host"="${NAMESPACE}-geth-ws.${INGRESS_BASE_DOMAIN}" \
--set "ingress.annotation_group_name"="${NAMESPACE}" \
--set "ingress.enabled"=true \
--set "networkPolicyDefault.ingress.allowCustomCidrs"=true \
--set "networkPolicyDefault.ingress.customCidrs"="${INGRESS_CIDRS}"
# to override default chain id uncomment the following line
# --set "geth.networkId"="2337"
```
