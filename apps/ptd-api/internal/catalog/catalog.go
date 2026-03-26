package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type Contract struct {
	ID             string            `json:"id"`
	Title          string            `json:"title"`
	Domain         string            `json:"domain"`
	SQLFile        string            `json:"sql_file"`
	ReadOnly       bool              `json:"read_only"`
	DefaultFilters map[string]string `json:"default_filters"`
	PlannedFilters []string          `json:"planned_filters"`
	Limit          Limit             `json:"limit"`
	CacheTTL       int               `json:"cache_ttl_seconds"`
	Columns        []string          `json:"columns"`
}

type Limit struct {
	Default int `json:"default"`
	Max     int `json:"max"`
}

type Catalog struct {
	repoRoot    string
	contractDir string
	byID        map[string]Contract
}

func Load(repoRoot, contractDir string) (*Catalog, error) {
	entries, err := filepath.Glob(filepath.Join(contractDir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob contract files: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no contract files found in %s", contractDir)
	}

	byID := make(map[string]Contract, len(entries))
	for _, path := range entries {
		contract, err := loadContract(path)
		if err != nil {
			return nil, err
		}
		if err := validateContract(repoRoot, contract); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if _, exists := byID[contract.ID]; exists {
			return nil, fmt.Errorf("%s: duplicate dataset id %q", path, contract.ID)
		}
		byID[contract.ID] = contract
	}

	return &Catalog{
		repoRoot:    repoRoot,
		contractDir: contractDir,
		byID:        byID,
	}, nil
}

func (c *Catalog) ContractDir() string {
	return c.contractDir
}

func (c *Catalog) Count() int {
	return len(c.byID)
}

func (c *Catalog) All() []Contract {
	contracts := make([]Contract, 0, len(c.byID))
	for _, contract := range c.byID {
		contracts = append(contracts, contract)
	}

	slices.SortFunc(contracts, func(a, b Contract) int {
		return strings.Compare(a.ID, b.ID)
	})

	return contracts
}

func (c *Catalog) Get(id string) (Contract, bool) {
	contract, ok := c.byID[id]
	return contract, ok
}

func (c *Catalog) LoadSQL(id string) (string, Contract, error) {
	contract, ok := c.Get(id)
	if !ok {
		return "", Contract{}, fmt.Errorf("unknown dataset id %q", id)
	}

	sqlPath := filepath.Join(c.repoRoot, filepath.FromSlash(contract.SQLFile))
	content, err := os.ReadFile(sqlPath)
	if err != nil {
		return "", Contract{}, fmt.Errorf("read sql file %s: %w", sqlPath, err)
	}

	return strings.TrimSpace(string(content)), contract, nil
}

func loadContract(path string) (Contract, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Contract{}, fmt.Errorf("read contract file %s: %w", path, err)
	}

	var contract Contract
	if err := json.Unmarshal(content, &contract); err != nil {
		return Contract{}, fmt.Errorf("decode contract file %s: %w", path, err)
	}

	return contract, nil
}

func validateContract(repoRoot string, contract Contract) error {
	if contract.ID == "" {
		return fmt.Errorf("missing id")
	}
	if contract.Title == "" {
		return fmt.Errorf("missing title")
	}
	if contract.SQLFile == "" {
		return fmt.Errorf("missing sql_file")
	}
	if contract.Limit.Default <= 0 {
		return fmt.Errorf("limit.default must be > 0")
	}
	if contract.Limit.Max < contract.Limit.Default {
		return fmt.Errorf("limit.max must be >= limit.default")
	}
	if len(contract.Columns) == 0 {
		return fmt.Errorf("columns must not be empty")
	}

	sqlPath := filepath.Join(repoRoot, filepath.FromSlash(contract.SQLFile))
	if _, err := os.Stat(sqlPath); err != nil {
		return fmt.Errorf("sql_file %s is not readable: %w", sqlPath, err)
	}

	return nil
}
