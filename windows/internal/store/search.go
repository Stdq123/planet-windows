package store

import (
	"errors"
	"math"
	"sort"
	"strings"
	"unicode"

	"windows/internal/feeds"
)

type SearchResult struct {
	Title           string  `json:"title"`
	SourceName      string  `json:"sourceName"`
	Snippet         string  `json:"snippet"`
	SiteID          string  `json:"siteId,omitempty"`
	ArticleID       string  `json:"articleId,omitempty"`
	SubscriptionURL string  `json:"subscriptionUrl,omitempty"`
	ItemKey         string  `json:"itemKey,omitempty"`
	RelevanceScore  float64 `json:"relevanceScore,omitempty"`
	BM25Score       float64 `json:"bm25Score,omitempty"`
	VectorScore     float64 `json:"vectorScore,omitempty"`
}

func tokenizeText(text string) []string {
	lower := strings.ToLower(text)
	var tokens []string
	var asciiWord []rune
	var cjkRunes []rune
	flushASCII := func() {
		if len(asciiWord) > 0 {
			tokens = append(tokens, string(asciiWord))
			asciiWord = asciiWord[:0]
		}
	}
	flushCJK := func() {
		if len(cjkRunes) > 0 {
			for i := 0; i < len(cjkRunes); i++ {
				tokens = append(tokens, string(cjkRunes[i]))
				if i+1 < len(cjkRunes) {
					tokens = append(tokens, string(cjkRunes[i:i+2]))
				}
			}
			cjkRunes = cjkRunes[:0]
		}
	}
	for _, r := range lower {
		if isCJKRune(r) {
			flushASCII()
			cjkRunes = append(cjkRunes, r)
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			flushCJK()
			asciiWord = append(asciiWord, r)
		} else {
			flushASCII()
			flushCJK()
		}
	}
	flushASCII()
	flushCJK()
	return tokens
}

func isCJKRune(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0x3040 && r <= 0x30FF) ||
		(r >= 0xAC00 && r <= 0xD7AF)
}

const vectorDim = 128

func embedTokens(tokens []string) [vectorDim]float64 {
	var vec [vectorDim]float64
	if len(tokens) == 0 {
		return vec
	}
	for _, tok := range tokens {
		h := uint32(2166136261)
		for i := 0; i < len(tok); i++ {
			h ^= uint32(tok[i])
			h *= 16777619
		}
		idx := h % vectorDim
		sign := 1.0
		if (h>>16)&1 == 1 {
			sign = -1.0
		}
		vec[idx] += sign
	}
	var norm float64
	for _, v := range vec {
		norm += v * v
	}
	if norm > 0 {
		inv := 1.0 / math.Sqrt(norm)
		for i := range vec {
			vec[i] *= inv
		}
	}
	return vec
}

func cosineSim(a, b [vectorDim]float64) float64 {
	var dot float64
	for i := 0; i < vectorDim; i++ {
		dot += a[i] * b[i]
	}
	return dot
}

type searchCandidate struct {
	result SearchResult
	title  string
	body   string
	tokens []string
	tf     map[string]int
	vec    [vectorDim]float64
}

func makeSnippet(text string) string {
	chars := []rune(strings.Join(strings.Fields(text), " "))
	if len(chars) > 240 {
		return string(chars[:240]) + "…"
	}
	return string(chars)
}

func (s *Store) collectSearchCandidates() []searchCandidate {
	s.mu.Lock()
	articles := append([]Article(nil), s.data.Articles...)
	sites := append([]Site(nil), s.data.Sites...)
	subs := append([]Subscription(nil), s.data.Subscriptions...)
	for i := range subs {
		subs[i].Feed.Items = append(subs[i].Feed.Items[:0:0], subs[i].Feed.Items...)
	}
	s.mu.Unlock()

	names := map[string]string{}
	for _, site := range sites {
		if !site.Removed && !site.DeletionPending {
			names[site.ID] = site.Name
		}
	}
	var candidates []searchCandidate
	for _, article := range articles {
		name, ok := names[article.SiteID]
		if !ok {
			continue
		}
		full := article.Title + " " + article.Title + " " + article.Content
		tokens := tokenizeText(full)
		tf := make(map[string]int, len(tokens))
		for _, tok := range tokens {
			tf[tok]++
		}
		candidates = append(candidates, searchCandidate{
			result: SearchResult{
				Title:      article.Title,
				SourceName: name,
				Snippet:    makeSnippet(article.Content),
				SiteID:     article.SiteID,
				ArticleID:  article.ID,
			},
			title:  article.Title,
			body:   article.Content,
			tokens: tokens,
			tf:     tf,
			vec:    embedTokens(tokens),
		})
	}
	for _, sub := range subs {
		source := sub.CustomTitle
		if source == "" {
			source = sub.Feed.Title
		}
		for _, item := range sub.Feed.Items {
			body := item.Description
			if body == "" {
				body = item.Content
			}
			full := item.Title + " " + item.Title + " " + item.Description + " " + item.Content
			tokens := tokenizeText(full)
			tf := make(map[string]int, len(tokens))
			for _, tok := range tokens {
				tf[tok]++
			}
			candidates = append(candidates, searchCandidate{
				result: SearchResult{
					Title:           item.Title,
					SourceName:      source,
					Snippet:         makeSnippet(body),
					SubscriptionURL: sub.URL,
					ItemKey:         feeds.ItemKey(item),
				},
				title:  item.Title,
				body:   item.Description + " " + item.Content,
				tokens: tokens,
				tf:     tf,
				vec:    embedTokens(tokens),
			})
		}
	}
	return candidates
}

// SearchArticles performs BM25 + semantic vector Reciprocal Rank Fusion (RRF) search across active sites and subscriptions.
func (s *Store) SearchArticles(query string) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) > 200 {
		return nil, errors.New("搜索内容不能超过 200 个字符")
	}
	if query == "" {
		return []SearchResult{}, nil
	}
	lowerQuery := strings.ToLower(query)
	qTokens := tokenizeText(lowerQuery)
	qVec := embedTokens(qTokens)
	candidates := s.collectSearchCandidates()
	if len(candidates) == 0 {
		return []SearchResult{}, nil
	}

	var totalLen float64
	df := map[string]int{}
	for _, c := range candidates {
		totalLen += float64(len(c.tokens))
		seen := map[string]bool{}
		for _, tok := range qTokens {
			if c.tf[tok] > 0 && !seen[tok] {
				df[tok]++
				seen[tok] = true
			}
		}
	}
	avgLen := totalLen / float64(len(candidates))
	if avgLen < 1 {
		avgLen = 1
	}
	const k1 = 1.2
	const b = 0.75
	nDocs := float64(len(candidates))

	type scoredIdx struct {
		idx   int
		score float64
	}
	var bm25List []scoredIdx
	var vecList []scoredIdx
	matchedKeyword := make([]bool, len(candidates))

	for i, c := range candidates {
		substrMatch := strings.Contains(strings.ToLower(c.title), lowerQuery) || strings.Contains(strings.ToLower(c.body), lowerQuery)
		var bm25 float64
		docLen := float64(len(c.tokens))
		for _, tok := range qTokens {
			freq := float64(c.tf[tok])
			if freq == 0 {
				continue
			}
			docFreq := float64(df[tok])
			idf := math.Log(1.0 + (nDocs-docFreq+0.5)/(docFreq+0.5))
			tfNorm := (freq * (k1 + 1.0)) / (freq + k1*(1.0-b+b*(docLen/avgLen)))
			bm25 += idf * tfNorm
		}
		if substrMatch {
			bm25 += 2.0
		}
		if bm25 > 0 {
			matchedKeyword[i] = true
			bm25List = append(bm25List, scoredIdx{idx: i, score: bm25})
		}
		sim := cosineSim(qVec, c.vec)
		if sim > 0.25 && (matchedKeyword[i] || len(qTokens) > 1) {
			vecList = append(vecList, scoredIdx{idx: i, score: sim})
		}
	}

	sort.SliceStable(bm25List, func(i, j int) bool { return bm25List[i].score > bm25List[j].score })
	sort.SliceStable(vecList, func(i, j int) bool { return vecList[i].score > vecList[j].score })

	const rrfK = 60.0
	const bm25Weight = 1.5
	const vectorWeight = 1.0

	fusedScores := map[int]float64{}
	bm25Scores := map[int]float64{}
	vecScores := map[int]float64{}

	for rank, item := range bm25List {
		contrib := bm25Weight / (rrfK + float64(rank+1))
		fusedScores[item.idx] += contrib
		bm25Scores[item.idx] = item.score
	}
	for rank, item := range vecList {
		if !matchedKeyword[item.idx] && item.score < 0.55 {
			continue
		}
		contrib := vectorWeight / (rrfK + float64(rank+1))
		fusedScores[item.idx] += contrib
		vecScores[item.idx] = item.score
	}

	var fused []scoredIdx
	for idx, score := range fusedScores {
		fused = append(fused, scoredIdx{idx: idx, score: score})
	}
	sort.SliceStable(fused, func(i, j int) bool {
		if fused[i].score != fused[j].score {
			return fused[i].score > fused[j].score
		}
		return fused[i].idx < fused[j].idx
	})
	if len(fused) > 200 {
		fused = fused[:200]
	}
	results := make([]SearchResult, 0, len(fused))
	for _, item := range fused {
		res := candidates[item.idx].result
		res.RelevanceScore = item.score
		res.BM25Score = bm25Scores[item.idx]
		res.VectorScore = vecScores[item.idx]
		results = append(results, res)
	}
	return results, nil
}

// RelatedArticles returns up to limit semantically similar articles across sites and subscriptions.
func (s *Store) RelatedArticles(siteID, articleID, subscriptionURL, itemKey string, limit int) ([]SearchResult, error) {
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	candidates := s.collectSearchCandidates()
	targetIdx := -1
	for i, c := range candidates {
		if siteID != "" && articleID != "" && c.result.SiteID == siteID && c.result.ArticleID == articleID {
			targetIdx = i
			break
		}
		if subscriptionURL != "" && itemKey != "" && c.result.SubscriptionURL == subscriptionURL && c.result.ItemKey == itemKey {
			targetIdx = i
			break
		}
	}
	if targetIdx == -1 {
		return []SearchResult{}, nil
	}
	target := candidates[targetIdx]
	type scored struct {
		res   SearchResult
		score float64
	}
	var matches []scored
	for i, c := range candidates {
		if i == targetIdx {
			continue
		}
		sim := cosineSim(target.vec, c.vec)
		var overlap int
		for tok := range target.tf {
			if len(tok) > 1 && c.tf[tok] > 0 {
				overlap++
			}
		}
		score := sim + float64(overlap)*0.05
		if score > 0.12 {
			res := c.result
			res.RelevanceScore = score
			res.VectorScore = sim
			matches = append(matches, scored{res: res, score: score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].score > matches[j].score })
	if len(matches) > limit {
		matches = matches[:limit]
	}
	out := make([]SearchResult, 0, len(matches))
	for _, m := range matches {
		out = append(out, m.res)
	}
	return out, nil
}
