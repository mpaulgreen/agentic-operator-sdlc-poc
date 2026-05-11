/*
Copyright 2026.

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

package controller

import (
	"crypto/rand"
	"math/big"

	databasev1alpha1 "github.com/example/postgres-operator/api/v1alpha1"
)

const (
	passwordLength  = 24
	passwordCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

// labelsForPostgresCluster returns the standard labels for all resources managed by the operator.
func labelsForPostgresCluster(cr *databasev1alpha1.PostgresCluster) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "postgresql",
		"app.kubernetes.io/instance":   cr.Name,
		"app.kubernetes.io/managed-by": "postgres-operator",
		"app.kubernetes.io/part-of":    cr.Name,
		"app.kubernetes.io/version":    cr.Spec.Version,
	}
}

// generatePassword creates a cryptographically random password.
func generatePassword() string {
	result := make([]byte, passwordLength)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(passwordCharset))))
		if err != nil {
			// Fallback should never happen with crypto/rand, but be safe
			result[i] = passwordCharset[0]
			continue
		}
		result[i] = passwordCharset[n.Int64()]
	}
	return string(result)
}
