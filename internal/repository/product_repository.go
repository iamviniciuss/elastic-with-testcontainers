package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

type Product struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Category    string  `json:"category"`
	TenantID    string  `json:"tenant_id"`
}

type ProductRepository struct {
	client *elasticsearch.Client
}

func NewProductRepository(client *elasticsearch.Client) *ProductRepository {
	return &ProductRepository{
		client: client,
	}
}

func (r *ProductRepository) Create(ctx context.Context, product *Product) error {
	productJSON, err := json.Marshal(product)
	if err != nil {
		return fmt.Errorf("failed to marshal product: %w", err)
	}

	req := esapi.IndexRequest{
		Index:      "products",
		DocumentID: product.ID,
		Body:       strings.NewReader(string(productJSON)),
		Refresh:    "true",
	}

	res, err := req.Do(ctx, r.client)
	if err != nil {
		return fmt.Errorf("failed to index product: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("elasticsearch error: %s", res.Status())
	}

	return nil
}

func (r *ProductRepository) GetByID(ctx context.Context, id string, tenantID string) (*Product, error) {
	req := esapi.GetRequest{
		Index:      "products",
		DocumentID: id,
	}

	res, err := req.Do(ctx, r.client)
	if err != nil {
		return nil, fmt.Errorf("failed to get product: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == 404 {
		return nil, nil
	}

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch error: %s", res.Status())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	source, found := response["_source"]
	if !found {
		return nil, fmt.Errorf("product source not found")
	}

	sourceJSON, err := json.Marshal(source)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal source: %w", err)
	}

	var product Product
	if err := json.Unmarshal(sourceJSON, &product); err != nil {
		return nil, fmt.Errorf("failed to unmarshal product: %w", err)
	}

	// Validar tenantID para isolamento
	if product.TenantID != tenantID {
		return nil, nil // Não encontrado para este tenant
	}

	return &product, nil
}

func (r *ProductRepository) SearchByCategory(ctx context.Context, category string, tenantID string) ([]*Product, error) {
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"term": map[string]interface{}{
							"category.keyword": category,
						},
					},
					{
						"term": map[string]interface{}{
							"tenant_id.keyword": tenantID,
						},
					},
				},
			},
		},
	}

	queryJSON, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	req := esapi.SearchRequest{
		Index: []string{"products"},
		Body:  strings.NewReader(string(queryJSON)),
	}

	res, err := req.Do(ctx, r.client)
	if err != nil {
		return nil, fmt.Errorf("failed to search products: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch search error: %s", res.Status())
	}

	var searchResponse map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&searchResponse); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	hits, ok := searchResponse["hits"].(map[string]interface{})
	if !ok {
		return []*Product{}, nil
	}

	hitsArray, ok := hits["hits"].([]interface{})
	if !ok {
		return []*Product{}, nil
	}

	var products []*Product
	for _, hit := range hitsArray {
		hitMap, ok := hit.(map[string]interface{})
		if !ok {
			continue
		}

		source, ok := hitMap["_source"].(map[string]interface{})
		if !ok {
			continue
		}

		sourceJSON, err := json.Marshal(source)
		if err != nil {
			continue
		}

		var product Product
		if err := json.Unmarshal(sourceJSON, &product); err != nil {
			continue
		}

		products = append(products, &product)
	}

	return products, nil
}