package mapper

import (
	"sort"
	"strings"
)

// ClusterUnit is one ranked cluster with its assigned folder and span folders.
type ClusterUnit struct {
	Rank      int      `json:"rank"`
	Size      int      `json:"size"`
	Members   []int    `json:"members"`
	Folder    string   `json:"folder"`
	Spans     []string `json:"spans"`
	Multi     bool     `json:"multi"`
	Tightness float64  `json:"tightness"`
}

// Bin is a packed group of cluster indices belonging to one folder.
type Bin struct {
	Size     int   `json:"size"`
	Clusters []int `json:"clusters"`
}

// FolderBins holds the direct bins and standalone facts assigned to a folder.
type FolderBins struct {
	Bins       []Bin `json:"bins"`
	Standalone []int `json:"standalone"`
}

// FolderCounts aggregates facts, bins, clusters and standalone facts.
type FolderCounts struct {
	Facts      int `json:"facts"`
	Bins       int `json:"bins"`
	Clusters   int `json:"clusters"`
	Standalone int `json:"standalone"`
}

func (c *FolderCounts) add(other FolderCounts) {
	c.Facts += other.Facts
	c.Bins += other.Bins
	c.Clusters += other.Clusters
	c.Standalone += other.Standalone
}

// FolderNode is the typed folder tree produced by binning.
type FolderNode struct {
	Path       string       `json:"path"`
	Counts     FolderCounts `json:"counts"`
	Bins       []Bin        `json:"bins"`
	Standalone []int        `json:"standalone"`
	Children   []FolderNode `json:"children"`
}

// BinMetrics summarizes bin size quality.
type BinMetrics struct {
	BinOutsideIdealFraction float64            `json:"bin_outside_ideal_fraction"`
	BinHardSizeViolations   int                `json:"bin_hard_size_violations"`
	BinSizePercentiles      map[string]float64 `json:"bin_size_percentiles"`
}

// BinningResult is the full folder-binning output.
type BinningResult struct {
	Folders    map[string]FolderBins `json:"folders"`
	Sizes      []int                 `json:"sizes"`
	Units      []ClusterUnit         `json:"units"`
	Tree       FolderNode            `json:"tree"`
	Threshold  int                   `json:"threshold"`
	RootCounts FolderCounts          `json:"root_counts"`
	Metrics    BinMetrics            `json:"metrics"`
}

func folderOf(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	prefix := ""
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		prefix = path[:idx]
	}
	parts := make([]string, 0)
	for _, part := range strings.Split(prefix, "/") {
		switch part {
		case "", ".":
		case "..":
			if len(parts) > 0 && parts[len(parts)-1] != ".." {
				parts = parts[:len(parts)-1]
			} else {
				parts = append(parts, "..")
			}
		default:
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return "."
	}
	return strings.Join(parts, "/")
}

func parentFolder(folder string) string {
	if idx := strings.LastIndex(folder, "/"); idx >= 0 {
		p := folder[:idx]
		if p == "" {
			return "."
		}
		return p
	}
	return "."
}

func ancestors(folder string) []string {
	if folder == "." {
		return []string{"."}
	}
	parts := strings.Split(folder, "/")
	out := make([]string, 0, len(parts)+1)
	for i := 1; i <= len(parts); i++ {
		out = append(out, strings.Join(parts[:i], "/"))
	}
	out = append(out, ".")
	return out
}

func nearest(folder string, counts map[string]int, threshold int) string {
	for folder != "." && counts[folder] < threshold {
		folder = parentFolder(folder)
	}
	return folder
}

func deeperOrLexical(a, b string) bool {
	da := strings.Count(a, "/")
	db := strings.Count(b, "/")
	if da != db {
		return da > db
	}
	return a < b
}

// BuildBins assigns whole clusters to their most common source folder, pools
// small folder groups toward root, and packs largest-first into size-bounded
// bins. Standalone facts stay under their source folders.
func BuildBins(dataset *Dataset, result *PipelineResult, opt *BinOptions) (*BinningResult, error) {
	if err := dataset.Validate(); err != nil {
		return nil, err
	}
	if err := result.Options.Validate(); err != nil {
		return nil, err
	}
	if err := opt.Validate(&result.Options); err != nil {
		return nil, err
	}
	n := len(dataset.Facts)
	labels, err := Assignments(result.Domains, n)
	if err != nil {
		return nil, err
	}
	expected := make([]int, 0)
	for i, rank := range labels {
		if rank == -1 {
			expected = append(expected, i)
		}
	}
	actual := append([]int(nil), result.Leftovers...)
	sort.Ints(actual)
	if len(actual) != len(expected) {
		return nil, errNotPartitioned
	}
	for i := range actual {
		if actual[i] != expected[i] {
			return nil, errNotPartitioned
		}
	}
	for _, domain := range result.Domains {
		if !result.Options.ClusterSizes.Accepts(domain.Size()) || !opt.Sizes.Accepts(domain.Size()) {
			return nil, errClusterSizeViolation
		}
	}
	counts := map[string]int{}
	for _, domain := range result.Domains {
		for _, member := range domain.Members {
			for _, folder := range ancestors(folderOf(dataset.Facts[member].Path)) {
				counts[folder]++
			}
		}
	}
	threshold := opt.FolderPoolingThreshold
	units := make([]ClusterUnit, 0, len(result.Domains))
	groups := map[string][]int{}
	for rank, domain := range result.Domains {
		directories := make([]string, len(domain.Members))
		frequency := map[string]int{}
		for i, member := range domain.Members {
			directories[i] = folderOf(dataset.Facts[member].Path)
			frequency[directories[i]]++
		}
		primary := ""
		bestCount := -1
		for folder, count := range frequency {
			if count > bestCount || (count == bestCount && folder < primary) {
				primary = folder
				bestCount = count
			}
		}
		spanSet := map[string]struct{}{}
		for _, directory := range directories {
			spanSet[nearest(directory, counts, threshold)] = struct{}{}
		}
		spans := make([]string, 0, len(spanSet))
		for span := range spanSet {
			spans = append(spans, span)
		}
		sort.Strings(spans)
		folder := nearest(primary, counts, threshold)
		units = append(units, ClusterUnit{
			Rank:      rank + 1,
			Size:      domain.Size(),
			Members:   append([]int(nil), domain.Members...),
			Folder:    folder,
			Spans:     spans,
			Multi:     len(spans) > 1,
			Tightness: domain.Tightness,
		})
		groups[folder] = append(groups[folder], rank)
	}
	for {
		small := ""
		found := false
		for folder, ids := range groups {
			if folder == "." {
				continue
			}
			sum := 0
			for _, i := range ids {
				sum += units[i].Size
			}
			if sum >= threshold {
				continue
			}
			if !found || deeperOrLexical(folder, small) {
				small = folder
				found = true
			}
		}
		if !found {
			break
		}
		target := parentFolder(small)
		moved := groups[small]
		delete(groups, small)
		for _, i := range moved {
			units[i].Folder = target
		}
		groups[target] = append(groups[target], moved...)
	}
	standalone := map[string][]int{}
	for _, i := range result.Leftovers {
		folder := folderOf(dataset.Facts[i].Path)
		standalone[folder] = append(standalone[folder], i)
	}
	folderNames := make([]string, 0, len(groups)+len(standalone))
	seen := map[string]struct{}{}
	for folder := range groups {
		seen[folder] = struct{}{}
		folderNames = append(folderNames, folder)
	}
	for folder := range standalone {
		if _, ok := seen[folder]; !ok {
			folderNames = append(folderNames, folder)
		}
	}
	sort.Strings(folderNames)
	folders := map[string]FolderBins{}
	sizes := make([]int, 0)
	for _, folder := range folderNames {
		ids := groups[folder]
		sort.SliceStable(ids, func(a, b int) bool {
			x, y := ids[a], ids[b]
			if units[x].Size != units[y].Size {
				return units[x].Size > units[y].Size
			}
			return x < y
		})
		packed := make([]Bin, 0)
		for _, i := range ids {
			best := -1
			bestSize := 0
			for p, bin := range packed {
				if bin.Size+units[i].Size > opt.Sizes.IdealMax {
					continue
				}
				if best == -1 || bin.Size > bestSize {
					best = p
					bestSize = bin.Size
				}
			}
			if best >= 0 {
				packed[best].Size += units[i].Size
				packed[best].Clusters = append(packed[best].Clusters, i)
			} else {
				packed = append(packed, Bin{Size: units[i].Size, Clusters: []int{i}})
			}
		}
		for {
			bestGain := -1
			bestA, bestB := -1, -1
			for a := 0; a < len(packed); a++ {
				for b := a + 1; b < len(packed); b++ {
					size := packed[a].Size + packed[b].Size
					smaller := packed[a].Size
					if packed[b].Size < smaller {
						smaller = packed[b].Size
					}
					if size > opt.Sizes.Max || smaller >= opt.Sizes.IdealMin {
						continue
					}
					before := 0
					if !opt.Sizes.Preferred(packed[a].Size) {
						before += packed[a].Size
					}
					if !opt.Sizes.Preferred(packed[b].Size) {
						before += packed[b].Size
					}
					after := 0
					if !opt.Sizes.Preferred(size) {
						after = size
					}
					gain := before - after
					if gain > 0 && gain > bestGain {
						bestGain = gain
						bestA = a
						bestB = b
					}
				}
			}
			if bestA < 0 {
				break
			}
			merged := packed[bestB]
			packed = append(packed[:bestB], packed[bestB+1:]...)
			packed[bestA].Size += merged.Size
			packed[bestA].Clusters = append(packed[bestA].Clusters, merged.Clusters...)
		}
		for index := range packed {
			clusters := packed[index].Clusters
			sort.SliceStable(clusters, func(a, b int) bool {
				x, y := clusters[a], clusters[b]
				if units[x].Size != units[y].Size {
					return units[x].Size > units[y].Size
				}
				return x < y
			})
			sizes = append(sizes, packed[index].Size)
		}
		directStandalone := standalone[folder]
		if directStandalone == nil {
			directStandalone = []int{}
		}
		folders[folder] = FolderBins{Bins: packed, Standalone: directStandalone}
	}
	aggregate := map[string]FolderCounts{}
	children := map[string]map[string]struct{}{}
	for folder, entries := range folders {
		direct := FolderCounts{
			Bins:       len(entries.Bins),
			Standalone: len(entries.Standalone),
		}
		for _, bin := range entries.Bins {
			direct.Facts += bin.Size
			direct.Clusters += len(bin.Clusters)
		}
		direct.Facts += len(entries.Standalone)
		for _, prefix := range ancestors(folder) {
			value := aggregate[prefix]
			value.add(direct)
			aggregate[prefix] = value
		}
		for _, prefix := range ancestors(folder) {
			if prefix == "." {
				continue
			}
			parent := parentFolder(prefix)
			if children[parent] == nil {
				children[parent] = map[string]struct{}{}
			}
			children[parent][prefix] = struct{}{}
		}
	}
	var buildTree func(path string) FolderNode
	buildTree = func(path string) FolderNode {
		direct := folders[path]
		bins := direct.Bins
		if bins == nil {
			bins = []Bin{}
		}
		standalone := direct.Standalone
		if standalone == nil {
			standalone = []int{}
		}
		node := FolderNode{
			Path:       path,
			Counts:     aggregate[path],
			Bins:       bins,
			Standalone: standalone,
			Children:   []FolderNode{},
		}
		kids := children[path]
		keys := make([]string, 0, len(kids))
		for kid := range kids {
			keys = append(keys, kid)
		}
		sort.Strings(keys)
		for _, kid := range keys {
			node.Children = append(node.Children, buildTree(kid))
		}
		return node
	}
	tree := buildTree(".")
	rootCounts := tree.Counts
	outside, violations, percentiles := sizeStats(sizes, opt.Sizes)
	return &BinningResult{
		Folders:    folders,
		Sizes:      sizes,
		Units:      units,
		Tree:       tree,
		Threshold:  threshold,
		RootCounts: rootCounts,
		Metrics: BinMetrics{
			BinOutsideIdealFraction: outside,
			BinHardSizeViolations:   violations,
			BinSizePercentiles:      percentiles,
		},
	}, nil
}
