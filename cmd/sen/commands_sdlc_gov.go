// Tệp commands_sdlc_gov.go cài đặt các lệnh quản trị SSDS: review (§4.3), approve (§21), impact (§17), exception (§27) và version (Phần II).
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/ssds"
	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// sdlcReview xác thực tách biệt vai trò phản biện độc lập (§4.3/§18) và lưu biên bản REVIEW vào chuỗi băm ssds_governance.
func sdlcReview(cwd string, args []string, stdout io.Writer) error {
	reordered := reorderFlagsFirst(args, map[string]bool{
		"phase": true, "author": true, "reviewer": true, "mode": true, "ref": true, "verdict": true, "notes": true,
	})
	fs := flag.NewFlagSet("sdlc review", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	phaseFlag := fs.String("phase", "", "SDLC phase ID (e.g. P1, P2, P5, P6)")
	authorFlag := fs.String("author", "", "author engineering role")
	reviewerFlag := fs.String("reviewer", "", "reviewer engineering role")
	modeFlag := fs.String("mode", "", "INDEPENDENT_AI_REVIEW|SELF_REVIEW|AUTOMATED_VALIDATION")
	refFlag := fs.String("ref", "", "independent subagent conversation/session reference ID")
	verdictFlag := fs.String("verdict", "", "PASS|FAIL")
	notesFlag := fs.String("notes", "", "review findings and notes")
	if err := fs.Parse(reordered); err != nil {
		return err
	}

	root := resolveProjectRoot(cwd)
	st, err := openExistingStore(root)
	if err != nil {
		return err
	}
	defer st.Close()

	phaseID := strings.ToUpper(strings.TrimSpace(*phaseFlag))
	if phaseID == "" {
		if proj, _ := st.GetSSDSProject(); proj != nil {
			phaseID = proj.CurrentPhase
		}
	}
	verdict := strings.ToUpper(strings.TrimSpace(*verdictFlag))
	if verdict != "PASS" && verdict != "FAIL" {
		return fmt.Errorf("invalid --verdict %q (expected PASS or FAIL)", *verdictFlag)
	}

	hasSeparateRef := strings.TrimSpace(*refFlag) != ""
	effMode, warn, err := ssds.ValidateIndependentReview(phaseID, *authorFlag, *reviewerFlag, *modeFlag, hasSeparateRef)
	if err != nil {
		return err
	}
	payloadBytes, _ := json.Marshal(map[string]string{
		"subagent_ref": strings.TrimSpace(*refFlag),
		"notes":        strings.TrimSpace(*notesFlag),
		"warning":      warn,
	})
	rec, err := st.AppendGovernanceRecord(storage.GovernanceRecord{
		RecordKind:     "REVIEW",
		PhaseID:        phaseID,
		AuthorityLevel: "MAJOR",
		AuthorRole:     strings.TrimSpace(*authorFlag),
		ReviewerRole:   strings.TrimSpace(*reviewerFlag),
		EvidenceMode:   effMode,
		Verdict:        verdict,
		PayloadJSON:    string(payloadBytes),
		Approved:       verdict == "PASS",
	})
	if err != nil {
		return err
	}
	_ = ssds.SyncProjectStateFiles(st, root)

	fmt.Fprintf(stdout, "Recorded %s [phase=%s mode=%s verdict=%s author=%q reviewer=%q]\n",
		rec.RecordID, rec.PhaseID, rec.EvidenceMode, rec.Verdict, rec.AuthorRole, rec.ReviewerRole)
	if warn != "" {
		fmt.Fprintf(stdout, "HONEST REVIEW DISCLOSURE: %s\n", warn)
	}
	return nil
}

// sdlcApprove thực thi phê duyệt tường minh của User (từ chối tuyệt đối câu trích dẫn rỗng theo điều khoản Silence != Approval §21).
func sdlcApprove(cwd string, args []string, stdout io.Writer) error {
	reordered := reorderFlagsFirst(args, map[string]bool{
		"phase": true, "record": true, "quote": true, "by": true,
	})
	fs := flag.NewFlagSet("sdlc approve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	phaseFlag := fs.String("phase", "", "phase ID to approve (e.g. P1, P2, P6, P7)")
	recordFlag := fs.String("record", "", "governance record ID to approve (e.g. CIA-001)")
	quoteFlag := fs.String("quote", "", "verbatim user approval quote")
	byFlag := fs.String("by", "User", "approver identity")
	if err := fs.Parse(reordered); err != nil {
		return err
	}

	root := resolveProjectRoot(cwd)
	st, err := openExistingStore(root)
	if err != nil {
		return err
	}
	defer st.Close()

	effQuote, err := resolveApprovalQuote(fs, *quoteFlag)
	if err != nil {
		return err
	}

	phaseID := strings.ToUpper(strings.TrimSpace(*phaseFlag))
	recID := strings.TrimSpace(*recordFlag)
	if phaseID == "" && recID == "" {
		if proj, _ := st.GetSSDSProject(); proj != nil {
			phaseID = proj.CurrentPhase
		}
	}
	if phaseID == "" && recID == "" {
		return errors.New("specify --phase <P> and/or --record <ID>")
	}

	if recID != "" {
		if err := st.ApproveGovernanceRecord(recID, effQuote); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Approved governance record %s [by=%s]\n", recID, strings.TrimSpace(*byFlag))
	}
	if phaseID != "" {
		if err := ssds.ApprovePhase(st, root, phaseID, *byFlag, effQuote); err != nil {
			return err
		}
		proj, _ := st.GetSSDSProject()
		nextPhase := phaseID
		if proj != nil {
			nextPhase = proj.CurrentPhase
		}
		fmt.Fprintf(stdout, "Approved phase %s -> COMPLETED [by=%s | active_phase=%s]\n",
			phaseID, strings.TrimSpace(*byFlag), nextPhase)
	}
	_ = ssds.SyncProjectStateFiles(st, root)
	return nil
}

// resolveApprovalQuote yêu cầu truyền tường minh --quote không rỗng trên CLI (cấm đọc ngầm last_prompt.txt, SSDS §21).
func resolveApprovalQuote(fs *flag.FlagSet, explicitQuote string) (string, error) {
	if !flagVisited(fs, "quote") {
		return "", errors.New("SSDS §21 violation: silence is never approval — explicit non-empty --quote is required")
	}
	q := strings.TrimSpace(explicitQuote)
	if q == "" {
		return "", errors.New("SSDS §21 violation: silence is never approval — empty --quote rejected")
	}
	return q, nil
}

// sdlcImpact ghi nhận hồ sơ phân tích tác động thay đổi đủ 10 chiều (§17) cho một tài liệu đích cụ thể (--doc).
func sdlcImpact(cwd string, args []string, stdout io.Writer) error {
	reordered := reorderFlagsFirst(args, map[string]bool{
		"phase": true, "level": true, "summary": true, "doc": true, "reqs": true, "adrs": true,
		"components": true, "modules": true, "db": true, "apis": true, "tests": true, "docs": true,
		"compat": true, "rollback": true, "quote": true,
	})
	fs := flag.NewFlagSet("sdlc impact", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	phaseFlag := fs.String("phase", "", "SDLC phase ID")
	levelFlag := fs.String("level", "MAJOR", "MINOR|MAJOR|CRITICAL")
	summaryFlag := fs.String("summary", "", "change impact summary")
	docFlag := fs.String("doc", "", "specific baseline document path (required, e.g. docs/02-architecture.md)")
	reqsFlag := fs.String("reqs", "", "affected requirement IDs")
	adrsFlag := fs.String("adrs", "", "affected ADR IDs")
	compFlag := fs.String("components", "", "affected architecture components")
	modFlag := fs.String("modules", "", "affected source modules")
	dbFlag := fs.String("db", "", "database schema/migration impact")
	apisFlag := fs.String("apis", "", "API contract impact")
	testsFlag := fs.String("tests", "", "test suite impact")
	docsFlag := fs.String("docs", "", "affected documentation files")
	compatFlag := fs.String("compat", "", "backward compatibility risks")
	rollbackFlag := fs.String("rollback", "", "migration and rollback plan")
	approveFlag := fs.Bool("approve", false, "user approved CRITICAL change impact")
	quoteFlag := fs.String("quote", "", "verbatim user approval quote for CRITICAL impact")
	if err := fs.Parse(reordered); err != nil {
		return err
	}

	root := resolveProjectRoot(cwd)
	st, err := openExistingStore(root)
	if err != nil {
		return err
	}
	defer st.Close()

	lvl := strings.ToUpper(strings.TrimSpace(*levelFlag))
	if lvl != "MINOR" && lvl != "MAJOR" && lvl != "CRITICAL" {
		return fmt.Errorf("invalid --level %q (expected MINOR|MAJOR|CRITICAL)", *levelFlag)
	}
	if strings.TrimSpace(*summaryFlag) == "" {
		return errors.New("missing --summary for change impact analysis")
	}
	if strings.TrimSpace(*docFlag) == "" {
		return errors.New("SSDS §17 violation: missing --doc (specify exact baseline document path, e.g. docs/02-architecture.md)")
	}
	phaseID := strings.ToUpper(strings.TrimSpace(*phaseFlag))
	if phaseID == "" {
		if proj, _ := st.GetSSDSProject(); proj != nil {
			phaseID = proj.CurrentPhase
		}
	}

	approved := lvl != "CRITICAL"
	effQuote := strings.TrimSpace(*quoteFlag)
	if *approveFlag {
		q, qErr := resolveApprovalQuote(fs, *quoteFlag)
		if qErr != nil {
			return qErr
		}
		effQuote = q
		approved = true
	}

	spec := ssds.ChangeImpactSpec{
		Summary:                    strings.TrimSpace(*summaryFlag),
		TargetDocument:             strings.TrimSpace(*docFlag),
		AffectedRequirements:       splitCSV(*reqsFlag),
		AffectedADRs:               splitCSV(*adrsFlag),
		AffectedComponents:         splitCSV(*compFlag),
		AffectedModules:            splitCSV(*modFlag),
		AffectedDatabase:           strings.TrimSpace(*dbFlag),
		AffectedAPIs:               strings.TrimSpace(*apisFlag),
		AffectedTests:              splitCSV(*testsFlag),
		AffectedDocs:               splitCSV(*docsFlag),
		BackwardCompatibilityRisks: strings.TrimSpace(*compatFlag),
		MigrationAndRollback:       strings.TrimSpace(*rollbackFlag),
	}
	rec, err := ssds.RecordChangeImpact(st, phaseID, lvl, "Principal Software Architect", spec, approved, effQuote)
	if err != nil {
		return err
	}
	_ = ssds.SyncProjectStateFiles(st, root)
	fmt.Fprintf(stdout, "Recorded %s [CHANGE_IMPACT level=%s phase=%s approved=%v]\n",
		rec.RecordID, rec.AuthorityLevel, rec.PhaseID, rec.Approved)
	return nil
}

// sdlcException ghi nhận hồ sơ ngoại lệ có thời hạn đủ 7 trường bắt buộc theo điều khoản SSDS §27.
func sdlcException(cwd string, args []string, stdout io.Writer) error {
	reordered := reorderFlagsFirst(args, map[string]bool{
		"id": true, "phase": true, "req": true, "reason": true,
		"risk": true, "mitigation": true, "approval": true, "expires": true, "level": true,
	})
	fs := flag.NewFlagSet("sdlc exception", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	idFlag := fs.String("id", "", "exception ID (e.g. EXC-001)")
	phaseFlag := fs.String("phase", "P1", "SDLC phase ID")
	reqFlag := fs.String("req", "", "affected requirement or SSDS clause")
	reasonFlag := fs.String("reason", "", "technical/business reason for exception")
	riskFlag := fs.String("risk", "", "risk assessment")
	mitigationFlag := fs.String("mitigation", "", "compensating mitigation")
	approvalFlag := fs.String("approval", "", "explicit approval quote")
	expiresFlag := fs.String("expires", "", "RFC3339 or YYYY-MM-DD expiration timestamp")
	levelFlag := fs.String("level", "MAJOR", "MINOR|MAJOR|CRITICAL")
	if err := fs.Parse(reordered); err != nil {
		return err
	}

	root := resolveProjectRoot(cwd)
	st, err := openExistingStore(root)
	if err != nil {
		return err
	}
	defer st.Close()

	rec, err := ssds.RecordException(st, strings.ToUpper(strings.TrimSpace(*phaseFlag)), strings.ToUpper(strings.TrimSpace(*levelFlag)), "Technical Lead", ssds.ExceptionSpec{
		ExceptionID:         strings.TrimSpace(*idFlag),
		Reason:              strings.TrimSpace(*reasonFlag),
		AffectedRequirement: strings.TrimSpace(*reqFlag),
		RiskAssessment:      strings.TrimSpace(*riskFlag),
		Mitigation:          strings.TrimSpace(*mitigationFlag),
		Approval:            strings.TrimSpace(*approvalFlag),
		Expiration:          strings.TrimSpace(*expiresFlag),
	})
	if err != nil {
		return err
	}
	_ = ssds.SyncProjectStateFiles(st, root)
	fmt.Fprintf(stdout, "Recorded exception %s [phase=%s expires=%s]\n", rec.RecordID, rec.PhaseID, rec.ExpiresAt)
	return nil
}

// sdlcVersion tra cứu lịch sử phiên bản SSDS, kiểm tra tương thích chỉ đọc, nâng cấp có kiểm soát và hoàn tác an toàn (Phần II).
func sdlcVersion(cwd string, args []string, stdout io.Writer) error {
	reordered := reorderFlagsFirst(args, map[string]bool{
		"check": true, "upgrade": true, "quote": true,
	})
	fs := flag.NewFlagSet("sdlc version", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	changelogFlag := fs.Bool("changelog", false, "print SSDS version changelog")
	checkFlag := fs.String("check", "", "run read-only upgrade compatibility check against target SSDS version")
	upgradeFlag := fs.String("upgrade", "", "perform controlled SSDS version upgrade (requires --approve and --quote)")
	approveFlag := fs.Bool("approve", false, "explicitly approve CRITICAL SSDS standard upgrade")
	quoteFlag := fs.String("quote", "", "verbatim user approval quote for SSDS upgrade")
	rollbackFlag := fs.Bool("rollback", false, "rollback to pre-upgrade SSDS version and state")
	if err := fs.Parse(reordered); err != nil {
		return err
	}

	if *changelogFlag {
		fmt.Fprint(stdout, ssds.ChangelogMarkdown())
		return nil
	}

	root := resolveProjectRoot(cwd)
	needsProjectDB := strings.TrimSpace(*checkFlag) != "" || strings.TrimSpace(*upgradeFlag) != "" || *rollbackFlag
	if !needsProjectDB && !fileExists(filepath.Join(root, ".sen", "state.db")) {
		fmt.Fprintf(stdout, "Sen-VN: v%s | Pinned SSDS Version: uninitialized | Bundled/Available: %v\n", workspace.CurrentVersion, ssds.ListBundledVersions())
		return nil
	}
	st, err := openExistingStore(root)
	if err != nil {
		return err
	}
	defer st.Close()

	if strings.TrimSpace(*checkFlag) != "" {
		rep, cErr := ssds.CheckUpgradeCompatibility(st, root, strings.TrimSpace(*checkFlag))
		if cErr != nil {
			return cErr
		}
		fmt.Fprintf(stdout, "[SSDS Compatibility Check: v%s -> v%s] Compatible=%v\n", rep.CurrentVersion, rep.TargetVersion, rep.Compatible)
		for _, bc := range rep.BreakingDiffs {
			fmt.Fprintf(stdout, "  BREAKING: %s\n", bc)
		}
		for _, md := range rep.MissingDeliverables {
			fmt.Fprintf(stdout, "  MISSING DELIVERABLE: %s\n", md)
		}
		for _, ms := range rep.MissingSections {
			fmt.Fprintf(stdout, "  MISSING SECTION: %s\n", ms)
		}
		return nil
	}
	if strings.TrimSpace(*upgradeFlag) != "" {
		return executeControlledVersionUpgrade(st, root, fs, strings.TrimSpace(*upgradeFlag), *approveFlag, *quoteFlag, stdout)
	}
	if *rollbackFlag {
		restored, rErr := ssds.RollbackSSDSUpgrade(st, root)
		if rErr != nil {
			return rErr
		}
		cfg, cErr := loadConfig(root)
		if cErr != nil {
			return cErr
		}
		cfg.SSDSVersion = restored
		if err := saveConfig(root, cfg); err != nil {
			return err
		}
		_ = ssds.SyncProjectStateFiles(st, root)
		fmt.Fprintf(stdout, "Rolled back SSDS version to v%s (signed .sen/config.json updated)\n", restored)
		return nil
	}

	proj, _ := st.GetSSDSProject()
	pinned := "uninitialized"
	if proj != nil {
		pinned = "v" + proj.AppliedSSDSVersion
	}
	fmt.Fprintf(stdout, "Pinned SSDS Version: %s | Bundled/Available: %v\n", pinned, ssds.ListBundledVersions())
	return nil
}

// executeControlledVersionUpgrade cưỡng chế phê duyệt tường minh (§21 CRITICAL) trước khi nâng cấp phiên bản SSDS và ký lại config.json.
func executeControlledVersionUpgrade(st *storage.Store, root string, fs *flag.FlagSet, targetVer string, approve bool, quote string, stdout io.Writer) error {
	if !approve {
		return errors.New("SSDS Part II / §21 violation: controlled SSDS version upgrade is a CRITICAL decision — pass --approve and non-empty --quote")
	}
	effQuote, err := resolveApprovalQuote(fs, quote)
	if err != nil {
		return err
	}
	if err := ssds.PerformControlledUpgrade(st, root, targetVer, effQuote); err != nil {
		return err
	}
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	cfg.SSDSVersion = strings.TrimPrefix(targetVer, "v")
	if err := saveConfig(root, cfg); err != nil {
		return err
	}
	_ = ssds.SyncProjectStateFiles(st, root)
	fmt.Fprintf(stdout, "Upgraded project SSDS standard to v%s (signed .sen/config.json updated)\n", cfg.SSDSVersion)
	return nil
}
