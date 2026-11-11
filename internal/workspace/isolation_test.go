// Tệp isolation_test.go kiểm chứng cô lập đa dự án trong cùng Workspace cha, chặn vượt biên giới dự án con và lọc mã nguồn.
package workspace_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// TestResolveNearestProjectRootZeroCrossContamination kiểm chứng định vị chính xác từng dự án con mà không rò rỉ sang dự án khác hoặc Workspace mẹ.
func TestResolveNearestProjectRootZeroCrossContamination(t *testing.T) {
	umbrella := t.TempDir()
	// Giả lập Workspace mẹ cũng vô tình có .sen/state.db riêng.
	writeTestFile(t, umbrella, ".sen/state.db", []byte("umbrella-db"))

	// Dự án A đã khởi tạo .sen/state.db và có thư mục con frontend/package.json (monorepo).
	projA := filepath.Join(umbrella, "Code", "Project-ERP")
	writeTestFile(t, projA, ".sen/state.db", []byte("erp-db"))
	writeTestFile(t, projA, ".git/HEAD", []byte("ref: refs/heads/main\n"))
	writeTestFile(t, projA, "frontend/package.json", []byte(`{"name":"erp-web"}`))

	// Dự án B đã khởi tạo .sen/state.db riêng.
	projB := filepath.Join(umbrella, "Code", "Project-TradingBot")
	writeTestFile(t, projB, ".sen/state.db", []byte("bot-db"))

	// Dự án C có kho .git riêng nhưng CHƯA khởi tạo .sen/state.db -> cấm rò rỉ ngược lên umbrella/.sen/state.db!
	projC := filepath.Join(umbrella, "Code", "Project-UninitGit")
	writeTestFile(t, projC, ".git", []byte("gitdir: ../.git/modules/c\n"))

	// 1. Thao tác lên tệp con chưa tồn tại trong Project-ERP/frontend/src/App.tsx -> phải trả về đúng (projA, true).
	gotRoot, hasDB := workspace.ResolveNearestProjectRoot("Code/Project-ERP/frontend/src/App.tsx", umbrella)
	if gotRoot != projA || !hasDB {
		t.Fatalf("expected (%s, true) for Project-ERP monorepo file, got (%s, %v)", projA, gotRoot, hasDB)
	}

	// 2. Thao tác lên Project-TradingBot/engine/order.go -> phải trả về đúng (projB, true).
	gotB, hasB := workspace.ResolveNearestProjectRoot(filepath.Join(projB, "engine", "order.go"), umbrella)
	if gotB != projB || !hasB {
		t.Fatalf("expected (%s, true) for Project-TradingBot, got (%s, %v)", projB, gotB, hasB)
	}

	// 3. Thao tác lên Project-UninitGit/cmd/main.go -> phải dừng ngay tại ranh giới .git (projC, false)!
	gotC, hasC := workspace.ResolveNearestProjectRoot(filepath.Join(projC, "cmd", "main.go"), umbrella)
	if gotC != projC || hasC {
		t.Fatalf("expected git boundary (%s, false) without leaking to umbrella .sen, got (%s, %v)", projC, gotC, hasC)
	}

	// 4. Đường dẫn nằm trong .agents/skills/... của projA -> tự động bước thoát .agents về đúng (projA, true).
	gotAg, hasAg := workspace.ResolveNearestProjectRoot(filepath.Join(projA, ".agents", "skills", "custom", "SKILL.md"), umbrella)
	if gotAg != projA || !hasAg {
		t.Fatalf("expected .agents path inside projA to resolve to (%s, true), got (%s, %v)", projA, gotAg, hasAg)
	}

	// 5. Dự án mới chỉ có manifest Cargo.toml (không có .git, không có .sen ở cả cha lẫn con).
	cleanWorkspace := t.TempDir()
	rustDir := filepath.Join(cleanWorkspace, "rust-cli")
	writeTestFile(t, rustDir, "Cargo.toml", []byte("[package]\nname = \"cli\"\n"))
	gotRust, hasRust := workspace.ResolveNearestProjectRoot("rust-cli/src/lib.rs", cleanWorkspace)
	if gotRust != rustDir || hasRust {
		t.Fatalf("expected standalone manifest boundary (%s, false), got (%s, %v)", rustDir, gotRust, hasRust)
	}
}

// TestSubprojectBoundaryGuardBlocksNestedCrossing kiểm chứng BeginTaskSnapshotWithOpts và CheckNoSubprojectCrossing chặn xâm phạm dự án lồng nhau.
func TestSubprojectBoundaryGuardBlocksNestedCrossing(t *testing.T) {
	outer := t.TempDir()
	st, err := storage.Open(filepath.Join(outer, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	writeTestFile(t, outer, "outer.go", []byte("package outer\n"))
	writeTestFile(t, outer, "services/nested-git/.git/HEAD", []byte("ref: refs/heads/main\n"))
	writeTestFile(t, outer, "services/nested-git/secret.go", []byte("package nested\n"))
	writeTestFile(t, outer, "services/nested-sen/.sen/state.db", []byte("nested-sen-db"))
	writeTestFile(t, outer, "services/nested-sen/app.py", []byte("print('isolated')\n"))

	if err := workspace.CaptureBaseline(st, outer); err != nil {
		t.Fatalf("CaptureBaseline outer failed: %v", err)
	}

	for _, crossPath := range []string{"services/nested-git/secret.go", "services/nested-sen/app.py"} {
		err := workspace.CheckNoSubprojectCrossing(outer, crossPath)
		if err == nil || !strings.Contains(err.Error(), "sub-project boundary violation") {
			t.Fatalf("expected sub-project boundary violation for %s, got: %v", crossPath, err)
		}
		errSnap := workspace.BeginTaskSnapshot(st, outer, "T-CROSS", []string{"outer.go", crossPath})
		if errSnap == nil || !strings.Contains(errSnap.Error(), "sub-project boundary violation") {
			t.Fatalf("expected BeginTaskSnapshot to block %s, got: %v", crossPath, errSnap)
		}
	}

	// Khi dự án con thay đổi tệp riêng bên trong nó, CheckIntegrity của dự án ngoài vẫn xanh 100% (0 nhiễm chéo)!
	writeTestFile(t, outer, "services/nested-git/new_feature.go", []byte("package nested\n"))
	if err := workspace.CheckIntegrity(st, outer, "", nil); err != nil {
		t.Fatalf("expected outer CheckIntegrity unaffected by isolated nested subproject edits, got: %v", err)
	}
}

// TestIsSoftwareSourceOrManifestPathClassification kiểm chứng bộ nhận diện mã nguồn/manifest và loại trừ .agents/.sen/docs.
func TestIsSoftwareSourceOrManifestPathClassification(t *testing.T) {
	mustTrue := []string{
		"cmd/sen/main.go", "src/app.py", "web/App.tsx", "web/index.ts", "lib/util.js",
		"ui/Card.jsx", "core/engine.rs", "Service.java", "Program.cs", "Main.kt",
		"App.swift", "lib/main.dart", "native/dsp.cpp", "native/dsp.c", "native/dsp.h",
		"db/001_init.sql", "ui/View.vue", "ui/Page.svelte", "api/index.php", "app/job.rb",
		"Dockerfile", "go.mod", "package.json", "pyproject.toml", "Cargo.toml",
		"pubspec.yaml", "pom.xml", "requirements.txt",
	}
	for _, p := range mustTrue {
		if !workspace.IsSoftwareSourceOrManifestPath(p) {
			t.Errorf("expected IsSoftwareSourceOrManifestPath(%q) == true", p)
		}
	}

	mustFalse := []string{
		"docs/00-project-charter.md", "docs/01-requirements.md", "PROJECT_STATUS.md",
		"notes.txt", ".sen/config.json", ".sen/state.db", ".agents/hooks.json",
		".agents/skills/archify/SKILL.md", ".agents/sen.lock.json", "edits/subtitles.srt",
	}
	for _, p := range mustFalse {
		if workspace.IsSoftwareSourceOrManifestPath(p) {
			t.Errorf("expected IsSoftwareSourceOrManifestPath(%q) == false", p)
		}
	}
}
