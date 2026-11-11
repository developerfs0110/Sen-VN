// Tệp detector.go thực thi phễu nhận diện tự động yêu cầu phần mềm song ngữ Việt-Anh (§1.3, §3) và chấm điểm 8 chiều rủi ro (§3.2).
package ssds

import (
	"strings"
	"unicode"
)

// DetectionResult chứa kết quả phân loại ý định phần mềm, 7 nhóm dự án, khung workflow gợi ý và 8 chiều rủi ro §3.2.
type DetectionResult struct {
	IsSoftwareRequest bool              `json:"is_software_request"`
	IsReadOnlyLookup  bool              `json:"is_read_only_lookup"`
	Category          string            `json:"category"`
	SuggestedWorkflow string            `json:"suggested_workflow"`
	RiskDimensions    map[string]string `json:"risk_dimensions"`
	HasExistingCode   bool              `json:"has_existing_code"`
	Reason            string            `json:"reason"`
}

// viRuneFoldMap ánh xạ toàn bộ nguyên âm/phụ âm tiếng Việt có dấu sang ký tự ASCII cơ sở để nhận diện không phân biệt dấu.
var viRuneFoldMap = map[rune]rune{
	'à': 'a', 'á': 'a', 'ả': 'a', 'ã': 'a', 'ạ': 'a',
	'ă': 'a', 'ằ': 'a', 'ắ': 'a', 'ẳ': 'a', 'ẵ': 'a', 'ặ': 'a',
	'â': 'a', 'ầ': 'a', 'ấ': 'a', 'ẩ': 'a', 'ẫ': 'a', 'ậ': 'a',
	'đ': 'd',
	'è': 'e', 'é': 'e', 'ẻ': 'e', 'ẽ': 'e', 'ẹ': 'e',
	'ê': 'e', 'ề': 'e', 'ế': 'e', 'ể': 'e', 'ễ': 'e', 'ệ': 'e',
	'ì': 'i', 'í': 'i', 'ỉ': 'i', 'ĩ': 'i', 'ị': 'i',
	'ò': 'o', 'ó': 'o', 'ỏ': 'o', 'õ': 'o', 'ọ': 'o',
	'ô': 'o', 'ồ': 'o', 'ố': 'o', 'ổ': 'o', 'ỗ': 'o', 'ộ': 'o',
	'ơ': 'o', 'ờ': 'o', 'ớ': 'o', 'ở': 'o', 'ỡ': 'o', 'ợ': 'o',
	'ù': 'u', 'ú': 'u', 'ủ': 'u', 'ũ': 'u', 'ụ': 'u',
	'ư': 'u', 'ừ': 'u', 'ứ': 'u', 'ử': 'u', 'ữ': 'u', 'ự': 'u',
	'ỳ': 'y', 'ý': 'y', 'ỷ': 'y', 'ỹ': 'y', 'ỵ': 'y',
}

// DetectSoftwareIntent phân tích ngữ cảnh thư mục và câu nhắc song ngữ Việt-Anh cho toàn bộ 15 trường hợp SSDS §1.3.
func DetectSoftwareIntent(projectRoot, userPrompt string) DetectionResult {
	res := DetectionResult{RiskDimensions: defaultLowRiskMap()}
	hasCode, domainProf := quickInspectSoftwareFootprint(projectRoot)
	res.HasExistingCode = hasCode

	folded := normalizeFoldedTokens(userPrompt)
	if folded == "" {
		return res
	}

	hasResearch := hasAnyPhrase(folded, []string{
		"feasibility", "technical investigation", "architecture study", "evaluate architecture",
		"nghien cuu kha thi", "danh gia kha thi", "khao sat ky thuat",
	})
	hasLookup := hasAnyPhrase(folded, []string{
		"explain", "what is", "what does", "how does", "why does", "why is", "describe", "summarize", "where is", "list all",
		"giai thich", "tai sao", "vi sao", "liet ke", "cho biet", "mo ta", "tom tat", "la gi", "chay the nao", "hoat dong the nao",
	})
	verbText := strings.NewReplacer(" lam luon di ", " ", " lam luon ", " ", " cu lam di ", " ").Replace(folded)
	hasBuildVerb := hasAnyPhrase(verbText, []string{
		"create", "build", "develop", "write", "implement", "code", "scaffold", "design", "make",
		"modify", "update", "edit", "add", "extend", "refactor", "optimize", "fix", "debug", "resolve", "patch",
		"migrate", "upgrade", "port", "integrate", "connect", "maintain", "prototype",
		"tao", "xay dung", "phat trien", "lap trinh", "viet", "dung", "lam", "thiet ke", "cai dat",
		"sua", "sua doi", "sua loi", "va loi", "khac phuc", "them", "bo sung", "mo rong",
		"tai cau truc", "toi uu", "nang cap", "chuyen doi", "di doi", "tich hop", "ket noi", "bao tri", "thu nghiem",
	})
	hasSoftDomain := hasAnyPhrase(folded, []string{
		"software", "app", "application", "website", "webapp", "web", "frontend", "backend", "api", "rest", "graphql", "grpc",
		"websocket", "socket", "webhook", "chat", "poc", "mvp", "daemon", "worker", "dashboard", "portal",
		"service", "microservice", "server", "cli", "desktop", "mobile", "android", "ios", "flutter", "react", "vue", "nextjs",
		"bot", "chatbot", "telegram", "discord", "automation", "crawler", "scraper", "script", "utility", "tool",
		"ai", "llm", "rag", "agent", "trading", "fintech", "financial", "payment", "wallet", "ledger", "erp", "crm", "saas",
		"database", "sql", "schema", "auth", "login", "oauth", "jwt", "endpoint", "module", "package", "sdk", "library",
		"function", "class", "component", "bug", "defect", "crash", "regression", "refactor", "system", "code", "repo",
		"phan mem", "ung dung", "trang web", "he thong", "dich vu", "cong cu", "tien ich", "co so du lieu",
		"giao dich", "tai chinh", "thanh toan", "ke toan", "chi tieu", "hoa don", "dang nhap", "xac thuc",
		"ma nguon", "tinh nang", "chuc nang", "loi",
	})
	hasPureMediaOrDocs := hasAnyPhrase(folded, []string{
		"mp4", "mkv", "mov", "srt", "ass", "subtitle", "ffmpeg", "montage", "trailer", "voiceover", "dubbing",
		"photoshop", "psd", "blender", "poem", "essay", "novel", "translate article",
		"dung phim", "cat ghep video", "phu de", "long tieng", "thiet ke anh", "bai tho", "bai van", "dich bai bao",
	})
	hasExplicitCodeArtifact := hasAnyPhrase(folded, []string{
		"software", "app", "application", "website", "webapp", "api", "backend", "frontend", "mobile", "desktop",
		"bot", "script", "code", "database", "sql", "trading", "plugin", "cli", "sdk",
		"phan mem", "ung dung", "trang web", "lap trinh", "ma nguon", "co so du lieu",
	})

	if hasResearch && (hasSoftDomain || hasCode) {
		res.IsSoftwareRequest = true
		res.Category = "RESEARCH"
		res.SuggestedWorkflow = scoreRiskAndSelectWorkflow(folded, res.Category, hasCode, res.RiskDimensions)
		res.Reason = "technical feasibility / software research request (§3.2 RESEARCH)"
		return res
	}
	if hasLookup && !hasBuildVerb {
		res.IsReadOnlyLookup = true
		res.Reason = "read-only explanation or lookup without software creation/modification verbs"
		return res
	}
	if (domainProf == "video" || domainProf == "media" || hasPureMediaOrDocs) && !hasExplicitCodeArtifact {
		res.Reason = "non-software media/video/documentation request"
		return res
	}
	if !hasBuildVerb || (!hasSoftDomain && !hasCode) {
		res.Reason = "no actionable software engineering intent detected"
		return res
	}

	res.IsSoftwareRequest = true
	res.Category = classifySoftwareCategory(folded, hasCode)
	res.SuggestedWorkflow = scoreRiskAndSelectWorkflow(folded, res.Category, hasCode, res.RiskDimensions)
	res.Reason = "matched SSDS §1.3 software lifecycle intent (" + res.Category + " -> " + res.SuggestedWorkflow + ")"
	return res
}

// classifySoftwareCategory phân loại yêu cầu vào đúng 1 trong 7 nhóm chuẩn của SSDS §3.2.
func classifySoftwareCategory(folded string, hasExistingCode bool) string {
	switch {
	case hasAnyPhrase(folded, []string{"prototype", "poc", "proof of concept", "nguyen mau", "thu nghiem nhanh", "mvp nhanh"}):
		return "PROTOTYPE"
	case hasAnyPhrase(folded, []string{"migrate", "migration", "upgrade", "port", "chuyen doi", "di doi", "nang cap"}):
		return "MIGRATION"
	case hasAnyPhrase(folded, []string{"integrate", "integration", "webhook", "external service", "external api", "third party", "stripe", "oauth", "tich hop", "ket noi api", "ket noi dich vu"}):
		return "INTEGRATION"
	case hasAnyPhrase(folded, []string{"fix", "bug", "defect", "crash", "hotfix", "patch", "maintain", "maintenance", "sua loi", "va loi", "khac phuc", "bao tri"}):
		return "MAINTENANCE"
	case hasExistingCode || hasAnyPhrase(folded, []string{"refactor", "existing", "extend", "modify", "add feature", "tai cau truc", "mo rong", "sua doi", "them tinh nang", "du an hien tai"}):
		return "BROWNFIELD"
	default:
		return "GREENFIELD"
	}
}

// scoreRiskAndSelectWorkflow đánh giá đủ 8 chiều rủi ro SSDS §3.2 và chọn WF_MICRO, WF_STANDARD hoặc WF_CRITICAL.
func scoreRiskAndSelectWorkflow(folded, category string, hasExistingCode bool, risks map[string]string) string {
	isCriticalDomain := category == "MIGRATION" || hasAnyPhrase(folded, []string{
		"trading", "financial", "fintech", "payment", "billing", "wallet", "ledger", "crypto", "exchange", "stop loss",
		"bank", "healthcare", "medical", "auth", "authentication", "authorization", "security", "encryption", "rbac",
		"production db", "delete data", "irreversible",
		"giao dich", "tai chinh", "thanh toan", "ngan hang", "cat lo", "y te", "xac thuc", "phan quyen", "bao mat", "xoa du lieu",
	})
	if isCriticalDomain {
		risks["complexity"] = "HIGH"
		risks["business_impact"] = "HIGH"
		risks["technical_risk"] = "HIGH"
		risks["security_sensitivity"] = "HIGH"
		risks["data_criticality"] = "HIGH"
		risks["deployment_risk"] = "HIGH"
		risks["regulatory_constraints"] = "MEDIUM"
		risks["reversibility"] = "HIGH"
		if hasAnyPhrase(folded, []string{"trading", "financial", "fintech", "payment", "bank", "healthcare", "giao dich", "tai chinh", "thanh toan", "ngan hang", "y te"}) {
			risks["regulatory_constraints"] = "HIGH"
		}
		return "WF_CRITICAL"
	}

	isMicro := hasAnyPhrase(folded, []string{
		"small script", "simple script", "utility script", "helper script", "one file", "minor bug", "typo",
		"script nho", "tien ich nho", "cong cu nho", "loi nho", "sua nho",
	}) || (hasAnyPhrase(folded, []string{"script"}) && !hasAnyPhrase(folded, []string{"system", "platform", "web", "app", "he thong"})) ||
		(category == "MAINTENANCE" && !hasAnyPhrase(folded, []string{"architecture", "database", "api", "kien truc", "co so du lieu"})) ||
		category == "RESEARCH"

	if isMicro {
		if hasExistingCode {
			risks["technical_risk"] = "LOW"
		}
		return "WF_MICRO"
	}

	risks["complexity"] = "MEDIUM"
	risks["business_impact"] = "MEDIUM"
	risks["technical_risk"] = "MEDIUM"
	risks["security_sensitivity"] = "MEDIUM"
	risks["data_criticality"] = "MEDIUM"
	risks["deployment_risk"] = "MEDIUM"
	return "WF_STANDARD"
}

// defaultLowRiskMap khởi tạo bản đồ đủ 8 chiều rủi ro chuẩn SSDS §3.2 ở mức mặc định LOW.
func defaultLowRiskMap() map[string]string {
	return map[string]string{
		"complexity":             "LOW",
		"business_impact":        "LOW",
		"technical_risk":         "LOW",
		"security_sensitivity":   "LOW",
		"data_criticality":       "LOW",
		"deployment_risk":        "LOW",
		"regulatory_constraints": "LOW",
		"reversibility":          "LOW",
	}
}

// normalizeFoldedTokens chuyển chuỗi sang chữ thường không dấu, thay dấu câu bằng khoảng trắng đơn và bọc khoảng trắng hai đầu.
func normalizeFoldedTokens(s string) string {
	var b strings.Builder
	b.WriteByte(' ')
	lastSpace := true
	for _, r := range strings.ToLower(s) {
		if mapped, ok := viRuneFoldMap[r]; ok {
			r = mapped
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastSpace = false
		} else if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	if !lastSpace {
		b.WriteByte(' ')
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		return ""
	}
	return " " + text + " "
}

// hasAnyPhrase kiểm tra sự xuất hiện theo ranh giới từ đầy đủ của bất kỳ từ/cụm từ nào trong văn bản đã chuẩn hóa.
func hasAnyPhrase(paddedFolded string, phrases []string) bool {
	for _, p := range phrases {
		if strings.Contains(paddedFolded, " "+p+" ") {
			return true
		}
	}
	return false
}
