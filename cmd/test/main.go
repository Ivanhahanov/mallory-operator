package main

import (
	"context"
	"fmt"

	v1 "k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func canI(k8sClient client.Client, verb, resource, namespace string) (bool, error) {
	review := &v1.SelfSubjectAccessReview{
		Spec: v1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &v1.ResourceAttributes{
				Verb:      verb,
				Resource:  resource,
				Namespace: namespace,
			},
		},
	}

	err := k8sClient.Create(context.TODO(), review)
	if err != nil {
		return false, err
	}

	return review.Status.Allowed, nil
}

func main() {
	// Создаём клиент для Kubernetes API
	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Println("Ошибка получения конфигурации Kubernetes:", err)
		return
	}

	k8sClient, err := client.New(cfg, client.Options{})
	if err != nil {
		fmt.Println("Ошибка создания клиента Kubernetes:", err)
		return
	}

	// Проверяем доступность действия
	allowed, err := canI(k8sClient, "create", "pods", "default")
	if err != nil {
		fmt.Println("Ошибка проверки доступа:", err)
		return
	}

	if allowed {
		fmt.Println("Вы можете создавать Pod в namespace 'default'")
	} else {
		fmt.Println("Вы НЕ можете создавать Pod в namespace 'default'")
	}
}
