lint:
	gofumpt -l -w .
	golangci-lint run

push-test:
	docker build -t ghcr.io/terricain/truenas-scale-csi:test .
	docker push ghcr.io/terricain/truenas-scale-csi:test

setup-test-namespace:
	kubectl apply -f temp/kubernetes/namespace.yaml
	kubectl apply -f temp/kubernetes/secret.yaml
	helm upgrade --install --namespace truenas-test --values temp/kubernetes/iscsi-values.yaml iscsi-csi charts/truenas-scale-csi


teardown-test-namespace:
	kubectl delete -f temp/kubernetes/namespace.yaml

.PHONY: lint push-test setup-test-namespace teardown-test-namespace
