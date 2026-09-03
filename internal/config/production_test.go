package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProductionPreflight(t *testing.T) {
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	caFile := filepath.Join(dir, "ca.pem")
	if err = os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"DEPLOYMENT_ENV": "production", "DATABASE_HOST": "db.example.test", "DATABASE_PORT": "5432", "DATABASE_NAME": "scheduler",
		"POSTGRES_SUPERUSER": "cluster_admin", "POSTGRES_SUPERUSER_PASSWORD": "cluster-admin-password-123456",
		"DATABASE_SSLMODE": "verify-full", "PGSSLROOTCERT": caFile, "ADMIN_COOKIE_SECURE": "true", "ADMIN_ACCESS_LOGIN_ENABLED": "false",
		"ADMIN_PUBLIC_URL": "https://admin.example.test", "SITE_PUBLIC_URL": "https://example.test", "ADMIN_METRICS_TOKEN": strings.Repeat("m", 32), "BOT_TOKEN": strings.Repeat("b", 40),
	}
	for _, role := range []string{"MIGRATOR", "BOT", "PARSER", "PRIVACY", "ADMIN", "SITE", "BACKUP", "RESTORE"} {
		values["DATABASE_"+role+"_USER"] = "scheduler_" + strings.ToLower(role)
		values["DATABASE_"+role+"_PASSWORD"] = role + strings.Repeat("x", 30)
	}
	get := func(key string) string { return values[key] }
	if err = ProductionPreflight(get); err != nil {
		t.Fatal(err)
	}
	for field, invalid := range map[string]string{
		"POSTGRES_SUPERUSER":          "",
		"POSTGRES_SUPERUSER_PASSWORD": "CHANGE_ME",
		"DATABASE_SSLMODE":            "require", "ADMIN_COOKIE_SECURE": "false", "ADMIN_ACCESS_LOGIN_ENABLED": "true",
		"ADMIN_PUBLIC_URL": "http://admin.example.test", "PGSSLROOTCERT": filepath.Join(dir, "missing"),
		"DATABASE_BOT_USER": "scheduler_admin", "DATABASE_BOT_PASSWORD": values["DATABASE_ADMIN_PASSWORD"],
		"DATABASE_MIGRATOR_PASSWORD": values["POSTGRES_SUPERUSER_PASSWORD"],
		"DATABASE_RESTORE_USER":      "cluster_admin", "DATABASE_SITE_USER": "scheduler_public_reader",
		"DATABASE_ADMIN_USER": "invalid-role-name",
	} {
		t.Run(field, func(t *testing.T) {
			previous := values[field]
			values[field] = invalid
			defer func() { values[field] = previous }()
			if err := ProductionPreflight(get); err == nil {
				t.Fatal("unsafe production setting accepted")
			}
		})
	}
}
