package catalog

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
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
	fsys        fs.FS
	contractDir string
	byID        map[string]Contract
}

func Load(fsys fs.FS, contractDir string) (*Catalog, error) {
	if fsys == nil {
		return nil, fmt.Errorf("catalog filesystem is not configured")
	}

	entries, err := fs.Glob(fsys, path.Join(contractDir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob contract files: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no contract files found in %s", contractDir)
	}

	byID := make(map[string]Contract, len(entries))
	for _, contractPath := range entries {
		content, err := fs.ReadFile(fsys, contractPath)
		if err != nil {
			return nil, fmt.Errorf("read contract file %s: %w", contractPath, err)
		}

		var contract Contract
		if err := json.Unmarshal(content, &contract); err != nil {
			return nil, fmt.Errorf("decode contract file %s: %w", contractPath, err)
		}

		if err := validateContract(fsys, contract); err != nil {
			return nil, fmt.Errorf("%s: %w", contractPath, err)
		}
		if _, exists := byID[contract.ID]; exists {
			return nil, fmt.Errorf("%s: duplicate dataset id %q", contractPath, contract.ID)
		}
		byID[contract.ID] = contract
	}

	return &Catalog{
		fsys:        fsys,
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

	content, err := fs.ReadFile(c.fsys, contract.SQLFile)
	if err != nil {
		return "", Contract{}, fmt.Errorf("read sql file %s: %w", contract.SQLFile, err)
	}

	return strings.TrimSpace(string(content)), contract, nil
}

func validateContract(fsys fs.FS, contract Contract) error {
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

	if _, err := fs.Stat(fsys, contract.SQLFile); err != nil {
		return fmt.Errorf("sql_file %s is not readable: %w", contract.SQLFile, err)
	}

	return nil
}
