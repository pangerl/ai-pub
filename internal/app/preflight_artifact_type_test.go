package app

import (
	"context"
	"testing"

	"ai-pub/internal/domain"
)

// TestPreflightOCIArtifactTypeBlock 覆盖 oci_image 部署目标的制品校验分支。
func TestPreflightOCIArtifactTypeBlock(t *testing.T) {
	ctx := context.Background()
	db, store := newReleaseTestStore(t)
	defer db.Close()
	service := NewReleaseService(store)
	fixture := createReleaseFixture(t, store)

	// 构造一个 oci_image 部署目标。
	ociTarget, err := store.CreateDeploymentTarget(ctx, domain.DeploymentTarget{
		ServiceID:     fixture.service.ID,
		EnvironmentID: fixture.testEnv.ID,
		ExecutorType:  "mock",
		TargetType:    "server",
		TargetRefID:   fixture.server.ID,
		ArtifactType:  "oci_image",
		EnvVars:       "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 版本无 artifact_url → block artifact_url_missing。
	noURLVersion, err := store.CreateServiceVersion(ctx, domain.ServiceVersion{ServiceID: fixture.service.ID, Version: "oci-no-url"})
	if err != nil {
		t.Fatal(err)
	}
	pf, err := service.Preflight(ctx, PreflightInput{
		ServiceID:         fixture.service.ID,
		EnvironmentID:     fixture.testEnv.ID,
		ServiceVersionID:  noURLVersion.ID,
		DeploymentTargetID: ociTarget.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pf.Result != "block" || !hasPreflightItem(pf, "artifact_url_missing", "block") {
		t.Fatalf("expected block artifact_url_missing for oci_image without artifact_url, got %#v", pf)
	}

	// artifact_url 为普通 tag（非 digest）→ block artifact_url_invalid。
	tagVersion, err := store.CreateServiceVersion(ctx, domain.ServiceVersion{
		ServiceID:   fixture.service.ID,
		Version:     "oci-tag",
		ArtifactURL: "harbor.example/team/order-api:latest",
	})
	if err != nil {
		t.Fatal(err)
	}
	pf, err = service.Preflight(ctx, PreflightInput{
		ServiceID:         fixture.service.ID,
		EnvironmentID:     fixture.testEnv.ID,
		ServiceVersionID:  tagVersion.ID,
		DeploymentTargetID: ociTarget.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pf.Result != "block" || !hasPreflightItem(pf, "artifact_url_invalid", "block") {
		t.Fatalf("expected block artifact_url_invalid for non-digest artifact_url, got %#v", pf)
	}

	// artifact_url 为合法 digest → pass。
	digestVersion, err := store.CreateServiceVersion(ctx, domain.ServiceVersion{
		ServiceID:   fixture.service.ID,
		Version:     "oci-digest",
		ArtifactURL: "harbor.example/team/order-api@sha256:" + "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
	})
	if err != nil {
		t.Fatal(err)
	}
	pf, err = service.Preflight(ctx, PreflightInput{
		ServiceID:         fixture.service.ID,
		EnvironmentID:     fixture.testEnv.ID,
		ServiceVersionID:  digestVersion.ID,
		DeploymentTargetID: ociTarget.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pf.Result != "pass" {
		t.Fatalf("expected pass for valid oci digest, got %#v", pf)
	}

	// version_only 部署目标缺 artifact_url → 仍为 warning，不阻断。
	voTarget, err := store.CreateDeploymentTarget(ctx, domain.DeploymentTarget{
		ServiceID:     fixture.service.ID,
		EnvironmentID: fixture.testEnv.ID,
		ExecutorType:  "mock",
		TargetType:    "server",
		TargetRefID:   fixture.server.ID,
		ArtifactType:  "version_only",
		EnvVars:       "{}",
	})
	if err != nil {
		t.Fatal(err)
	}
	pf, err = service.Preflight(ctx, PreflightInput{
		ServiceID:         fixture.service.ID,
		EnvironmentID:     fixture.testEnv.ID,
		ServiceVersionID:  noURLVersion.ID,
		DeploymentTargetID: voTarget.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pf.Result == "block" || !hasPreflightItem(pf, "artifact_url_missing", "warning") {
		t.Fatalf("expected warning (not block) for version_only without artifact_url, got %#v", pf)
	}
}
