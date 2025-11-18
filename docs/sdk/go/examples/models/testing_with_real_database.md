# 🧪 Testing with Real Database Connection

This guide demonstrates how to write unit tests using a real HydrAIDE database connection instead of mocks. 

## Why Test with Real Database?

When working with HydrAIDE, you can easily use a **real database connection** in your unit tests without the overhead of complex mocking setups. Since HydrAIDE is extremely lightweight and resource-efficient, maintaining a dedicated HydrAIDE instance for testing purposes is practical and cost-effective.

### Benefits

- ✅ **No mocking required** – Test against actual data operations
- ✅ **Real-world scenarios** – Catch issues that mocks might miss
- ✅ **Fast execution** – HydrAIDE's performance makes tests quick
- ✅ **Simple setup** – Just connect to your test instance
- ✅ **Accurate behavior** – Tests reflect production reality

### Important: Teardown and Cleanup

⚠️ **Critical Best Practice**: Always use the `Destroy()` method in your test teardown to remove test-created swamps. This ensures:

- Tests can be re-run without conflicts
- No leftover test data pollutes your test database
- Clean slate for each test execution

### Critical: Always Register Your Patterns

🔴 **Never forget to call `RegisterPattern()`** in your test setup! Without registering the pattern, HydrAIDE won't know about your swamp structure, and operations will fail.

```go
func (s *ProductTestSuite) SetupSuite() {
    // ... connect to repo ...
    
    // REQUIRED: Register the pattern
    model := &Product{}
    err := model.RegisterPattern(s.repoInterface)
    if err != nil {
        s.T().Fatalf("Failed to register pattern: %v", err)
    }
}
```

### Testing Beyond Models: Service Layer & Subscriptions

This approach isn't limited to just model testing! You can easily test your **service layer** with real data operations, including:

- ✅ **Business logic validation** – Test complex workflows with real database state
- ✅ **Subscription mechanisms** – Test reactive updates and event streams
- ✅ **Integration scenarios** – Test how multiple services interact with shared data
- ✅ **Real-time features** – Test WebSocket handlers, notification systems, etc.

**Example: Testing a service with subscriptions**

```go
func (s *ProductServiceTestSuite) TestProductUpdateNotification() {
    // Subscribe to product updates
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    updateReceived := make(chan bool, 1)
    
    go func() {
        err := s.productService.SubscribeToProductUpdates(ctx, "prod-123", func(product *Product) error {
            // Verify the update was received
            updateReceived <- true
            return nil
        })
        if err != nil {
            s.T().Logf("Subscription ended: %v", err)
        }
    }()
    
    // Give subscription time to initialize
    time.Sleep(100 * time.Millisecond)
    
    // Update the product (this should trigger the subscription)
    err := s.productService.UpdateProductPrice("prod-123", 199.99)
    assert.Nil(s.T(), err)
    
    // Wait for notification
    select {
    case <-updateReceived:
        s.T().Log("✅ Subscription received update successfully")
    case <-time.After(2 * time.Second):
        s.T().Fatal("❌ Subscription did not receive update")
    }
}
```

This way you can test end-to-end scenarios with real reactive behavior, not just mocked callbacks.

### Security: Never Hardcode Sensitive Data

🔒 **Important Security Practice**: Never put sensitive connection details directly in your test code!

❌ **DON'T DO THIS:**
```go
// Bad: Hardcoded credentials
s.repoInterface = repo.New([]*client.Server{
    {
        Host:          "production.example.com:50051",
        ClientCrtPath: "/home/user/secret/client.crt",
        ClientKeyPath: "/home/user/secret/client.key",
    },
}, 100, 4194304, false)
```

✅ **DO THIS INSTEAD:**
```go
// Good: Load from environment variables
func (s *ProductTestSuite) SetupSuite() {
    testHost := os.Getenv("HYDRAIDE_TEST_HOST")
    if testHost == "" {
        testHost = "localhost:50051" // Safe default for local testing
    }
    
    s.repoInterface = repo.New([]*client.Server{
        {
            Host:          testHost,
            FromIsland:    0,
            ToIsland:      99,
            CACrtPath:     os.Getenv("HYDRAIDE_TEST_CA_CRT"),
            ClientCrtPath: os.Getenv("HYDRAIDE_TEST_CLIENT_CRT"),
            ClientKeyPath: os.Getenv("HYDRAIDE_TEST_CLIENT_KEY"),
        },
    }, 100, 4194304, false)
    
    // ... rest of setup ...
}
```

**Example `.env` file for your CI/CD:**
```bash
HYDRAIDE_TEST_HOST=test-instance.internal:50051
HYDRAIDE_TEST_CA_CRT=/secrets/test-ca.crt
HYDRAIDE_TEST_CLIENT_CRT=/secrets/test-client.crt
HYDRAIDE_TEST_CLIENT_KEY=/secrets/test-client.key
```

---

## Test Suite Setup

We'll use the `testify/suite` package to organize our tests. This provides a clean structure with setup and teardown hooks.

### Basic Test Suite Structure

```go
package products_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/client"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// ProductTestSuite contains the test suite for Product model
type ProductTestSuite struct {
	suite.Suite
	repoInterface repo.Repo
}

// SetupSuite runs once before all tests in the suite
func (s *ProductTestSuite) SetupSuite() {
	// Connect to your test HydrAIDE instance
	s.repoInterface = repo.New([]*client.Server{
		{
			Host:          "localhost:50051",
			FromIsland:    0,
			ToIsland:      99,
			CACrtPath:     "/path/to/ca.crt",
			ClientCrtPath: "/path/to/client.crt",
			ClientKeyPath: "/path/to/client.key",
		},
	}, 100, // Total number of islands
		4194304, // Max message size (4MB)
		false)   // TLS enabled

	// Register the pattern for our model
	model := &Product{}
	err := model.RegisterPattern(s.repoInterface)
	assert.Nil(s.T(), err)

	// Clean up any existing test data from previous runs
	if err := model.Destroy(s.repoInterface); err != nil {
		// Log but don't fail if swamp doesn't exist
		s.T().Logf("Initial cleanup: %v", err)
	}
}

// TearDownSuite runs once after all tests in the suite complete
func (s *ProductTestSuite) TearDownSuite() {
	// Clean up all test data created during tests
	model := &Product{}
	err := model.Destroy(s.repoInterface)
	assert.Nil(s.T(), err, "Failed to destroy test swamps")
}

// TestProductTestSuite is the entry point for running the suite
func TestProductTestSuite(t *testing.T) {
	suite.Run(t, new(ProductTestSuite))
}
```

---

## Example 1: Profile Swamp Testing (CRUD Operations)

Profile swamps store individual entities with unique identifiers. Here's how to test them:

```go
// Product represents a product in our catalog
type Product struct {
	ProductID   string    `json:"product_id"`
	Name        string    `json:"name"`
	Price       float64   `json:"price"`
	Stock       int       `json:"stock"`
	Category    string    `json:"category"`
	CreatedAt   time.Time `json:"created_at"`
}

// RegisterPattern registers the swamp pattern for products
func (p *Product) RegisterPattern(r repo.Repo) error {
	return r.RegisterPattern("shop", "products", "*")
}

// Save stores the product in HydrAIDE
func (p *Product) Save(r repo.Repo) error {
	return r.SetTreasure("shop", "products", p.ProductID, "", p)
}

// Load retrieves the product from HydrAIDE
func (p *Product) Load(r repo.Repo) error {
	return r.GetTreasure("shop", "products", p.ProductID, "", p)
}

// Delete removes the product from HydrAIDE
func (p *Product) Delete(r repo.Repo) error {
	return r.DeleteTreasure("shop", "products", p.ProductID, "")
}

// Destroy removes all products (use in tests only!)
func (p *Product) Destroy(r repo.Repo) error {
	return r.DeleteSwamp("shop", "products", "*")
}

// TestSaveAndLoad tests basic Create and Read operations
func (s *ProductTestSuite) TestSaveAndLoad() {
	productID := uuid.New().String()
	
	// Create a test product
	testProduct := &Product{
		ProductID: productID,
		Name:      "Test Laptop",
		Price:     999.99,
		Stock:     50,
		Category:  "Electronics",
		CreatedAt: time.Now(),
	}

	// Save the product
	err := testProduct.Save(s.repoInterface)
	assert.Nil(s.T(), err, "Save should not return error")

	// Load the product back
	loadedProduct := &Product{
		ProductID: productID,
	}
	err = loadedProduct.Load(s.repoInterface)
	assert.Nil(s.T(), err, "Load should not return error")

	// Verify all fields match
	assert.Equal(s.T(), testProduct.ProductID, loadedProduct.ProductID)
	assert.Equal(s.T(), testProduct.Name, loadedProduct.Name)
	assert.Equal(s.T(), testProduct.Price, loadedProduct.Price)
	assert.Equal(s.T(), testProduct.Stock, loadedProduct.Stock)
	assert.Equal(s.T(), testProduct.Category, loadedProduct.Category)
	assert.False(s.T(), loadedProduct.CreatedAt.IsZero())

	s.T().Logf("Successfully saved and loaded product: %s", loadedProduct.Name)
}

// TestUpdate tests Update operations
func (s *ProductTestSuite) TestUpdate() {
	productID := uuid.New().String()
	
	// Create initial product
	product := &Product{
		ProductID: productID,
		Name:      "Original Name",
		Price:     100.00,
		Stock:     10,
		Category:  "Books",
		CreatedAt: time.Now(),
	}
	err := product.Save(s.repoInterface)
	assert.Nil(s.T(), err)

	// Update the product
	product.Name = "Updated Name"
	product.Price = 150.00
	product.Stock = 5
	err = product.Save(s.repoInterface)
	assert.Nil(s.T(), err)

	// Verify the update
	loadedProduct := &Product{ProductID: productID}
	err = loadedProduct.Load(s.repoInterface)
	assert.Nil(s.T(), err)
	
	assert.Equal(s.T(), "Updated Name", loadedProduct.Name)
	assert.Equal(s.T(), 150.00, loadedProduct.Price)
	assert.Equal(s.T(), 5, loadedProduct.Stock)

	s.T().Logf("Successfully updated product: %s", loadedProduct.Name)
}

// TestDelete tests Delete operations
func (s *ProductTestSuite) TestDelete() {
	productID := uuid.New().String()
	
	// Create a product
	testProduct := &Product{
		ProductID: productID,
		Name:      "To Be Deleted",
		Price:     50.00,
		Stock:     5,
		Category:  "Toys",
		CreatedAt: time.Now(),
	}
	err := testProduct.Save(s.repoInterface)
	assert.Nil(s.T(), err)

	// Verify it exists
	loadedProduct := &Product{ProductID: productID}
	err = loadedProduct.Load(s.repoInterface)
	assert.Nil(s.T(), err, "Product should exist before delete")

	// Delete the product
	err = testProduct.Delete(s.repoInterface)
	assert.Nil(s.T(), err, "Delete should not return error")

	// Verify it's gone
	err = loadedProduct.Load(s.repoInterface)
	assert.NotNil(s.T(), err, "Product should not exist after delete")

	s.T().Logf("Successfully deleted product: %s", testProduct.ProductID)
}
```

---

## Example 2: Catalog Swamp Testing (Expiration)

Catalog swamps are great for time-based operations. Here's an example with expiring items:

```go
// ProductDiscount represents a time-limited discount offer
type ProductDiscount struct {
	DiscountID string    `json:"discount_id"`
	ProductID  string    `json:"product_id"`
	Percentage float64   `json:"percentage"`
	ExpireAt   time.Time `json:"expire_at"`
}

// RegisterPattern registers the catalog pattern for discounts
func (d *ProductDiscount) RegisterPattern(r repo.Repo) error {
	return r.RegisterCatalogPattern("shop", "discounts", "*")
}

// Save stores the discount with expiration
func (d *ProductDiscount) Save(r repo.Repo) error {
	return r.SetCatalogItem("shop", "discounts", d.DiscountID, "", d, d.ExpireAt)
}

// Load retrieves a specific discount
func (d *ProductDiscount) Load(r repo.Repo) error {
	return r.GetCatalogItem("shop", "discounts", d.DiscountID, "", d)
}

// LoadExpired retrieves all expired discounts
func (d *ProductDiscount) LoadExpired(r repo.Repo, limit int) ([]*ProductDiscount, error) {
	var results []*ProductDiscount
	err := r.GetExpiredCatalogItems("shop", "discounts", "*", "", limit, &results)
	return results, err
}

// Delete removes a discount
func (d *ProductDiscount) Delete(r repo.Repo) error {
	return r.DeleteCatalogItem("shop", "discounts", d.DiscountID, "")
}

// Destroy removes all discounts (use in tests only!)
func (d *ProductDiscount) Destroy(r repo.Repo) error {
	return r.DeleteCatalogSwamp("shop", "discounts", "*")
}

// TestSaveAndLoadDiscount tests saving and loading discounts
func (s *ProductTestSuite) TestSaveAndLoadDiscount() {
	discountID := uuid.New().String()
	
	testDiscount := &ProductDiscount{
		DiscountID: discountID,
		ProductID:  "prod-123",
		Percentage: 25.0,
		ExpireAt:   time.Now().Add(24 * time.Hour), // Expires in 24 hours
	}

	err := testDiscount.Save(s.repoInterface)
	assert.Nil(s.T(), err, "Save should not return error")

	loadedDiscount := &ProductDiscount{
		DiscountID: discountID,
	}
	err = loadedDiscount.Load(s.repoInterface)
	assert.Nil(s.T(), err, "Load should not return error")

	assert.Equal(s.T(), testDiscount.DiscountID, loadedDiscount.DiscountID)
	assert.Equal(s.T(), testDiscount.ProductID, loadedDiscount.ProductID)
	assert.Equal(s.T(), testDiscount.Percentage, loadedDiscount.Percentage)
	assert.False(s.T(), loadedDiscount.ExpireAt.IsZero())

	s.T().Logf("Successfully saved and loaded discount: %s", loadedDiscount.DiscountID)
}

// TestLoadExpiredDiscounts tests the expiration mechanism
func (s *ProductTestSuite) TestLoadExpiredDiscounts() {
	// Create an expired discount
	expiredDiscount := &ProductDiscount{
		DiscountID: uuid.New().String(),
		ProductID:  "prod-expired",
		Percentage: 50.0,
		ExpireAt:   time.Now().Add(-5 * time.Second), // Already expired
	}
	err := expiredDiscount.Save(s.repoInterface)
	assert.Nil(s.T(), err)

	// Create a future discount
	futureDiscount := &ProductDiscount{
		DiscountID: uuid.New().String(),
		ProductID:  "prod-future",
		Percentage: 15.0,
		ExpireAt:   time.Now().Add(1 * time.Hour), // Not expired yet
	}
	err = futureDiscount.Save(s.repoInterface)
	assert.Nil(s.T(), err)

	// Wait for system to process expiration
	time.Sleep(6 * time.Second)

	// Load expired discounts
	model := &ProductDiscount{}
	expiredDiscounts, err := model.LoadExpired(s.repoInterface, 10)
	assert.Nil(s.T(), err)
	assert.Len(s.T(), expiredDiscounts, 1, "Should load exactly one expired discount")

	if len(expiredDiscounts) > 0 {
		assert.Equal(s.T(), expiredDiscount.DiscountID, expiredDiscounts[0].DiscountID)
		s.T().Logf("Successfully loaded expired discount: %s", expiredDiscounts[0].DiscountID)
	}

	// Verify expired discount was removed from catalog
	err = expiredDiscount.Load(s.repoInterface)
	assert.NotNil(s.T(), err, "Expired discount should be removed after LoadExpired")

	// Verify future discount still exists
	err = futureDiscount.Load(s.repoInterface)
	assert.Nil(s.T(), err, "Future discount should still exist")

	// Cleanup future discount
	err = futureDiscount.Delete(s.repoInterface)
	assert.Nil(s.T(), err)
}
```

---

## Example 3: Multiple Products Test

Testing bulk operations and ensuring proper cleanup:

```go
// TestMultipleProducts tests creating and managing multiple products
func (s *ProductTestSuite) TestMultipleProducts() {
	productIDs := []string{
		uuid.New().String(),
		uuid.New().String(),
		uuid.New().String(),
	}

	// Create multiple products
	products := []*Product{
		{
			ProductID: productIDs[0],
			Name:      "Product A",
			Price:     10.00,
			Stock:     100,
			Category:  "Category1",
			CreatedAt: time.Now(),
		},
		{
			ProductID: productIDs[1],
			Name:      "Product B",
			Price:     20.00,
			Stock:     50,
			Category:  "Category1",
			CreatedAt: time.Now(),
		},
		{
			ProductID: productIDs[2],
			Name:      "Product C",
			Price:     30.00,
			Stock:     25,
			Category:  "Category2",
			CreatedAt: time.Now(),
		},
	}

	// Save all products
	for _, product := range products {
		err := product.Save(s.repoInterface)
		assert.Nil(s.T(), err, "Failed to save product: %s", product.Name)
	}

	// Verify all products were saved
	for _, productID := range productIDs {
		loadedProduct := &Product{ProductID: productID}
		err := loadedProduct.Load(s.repoInterface)
		assert.Nil(s.T(), err, "Failed to load product: %s", productID)
		s.T().Logf("Loaded product: %s - %s", loadedProduct.ProductID, loadedProduct.Name)
	}

	// Delete all test products
	for _, product := range products {
		err := product.Delete(s.repoInterface)
		assert.Nil(s.T(), err, "Failed to delete product: %s", product.Name)
	}

	// Verify all products were deleted
	for _, productID := range productIDs {
		loadedProduct := &Product{ProductID: productID}
		err := loadedProduct.Load(s.repoInterface)
		assert.NotNil(s.T(), err, "Product should not exist after delete: %s", productID)
	}

	s.T().Log("Successfully tested multiple products lifecycle")
}
```

---

## Best Practices Summary

### ✅ DO

- **Use dedicated test HydrAIDE instance** – Separate from production
- **Always call `RegisterPattern()` in setup** – Required before any swamp operations
- **Always call `Destroy()` in `TearDownSuite`** – Clean up test swamps
- **Load config from environment variables** – Never hardcode sensitive data
- **Use unique IDs** – `uuid.New().String()` for test data
- **Use `testify/suite`** – Organized test structure with hooks
- **Test real scenarios** – Create, Read, Update, Delete operations
- **Test service layer too** – Including subscriptions and reactive logic
- **Clean up within tests** – Delete created items when needed
- **Add descriptive logs** – `s.T().Logf()` for debugging

### ❌ DON'T

- **Don't test against production** – Always use test instances
- **Don't forget `RegisterPattern()`** – Tests will fail without it
- **Don't hardcode credentials** – Use environment variables or config files
- **Don't forget teardown** – Always clean up test data
- **Don't reuse IDs** – Generate unique IDs for each test
- **Don't ignore errors** – Always assert error conditions
- **Don't leave test data** – Clean up after each test
- **Don't commit sensitive data** – Keep `.env` files out of version control

---

## Configuration Example

Here's how you should configure your test instance connection using environment variables:

```go
// test_config.go
package config

import (
	"os"
	"strconv"
	
	"github.com/hydraide/hydraide/sdk/go/hydraidego/client"
)

// GetTestRepoServers returns server configuration for testing
// Loads sensitive data from environment variables
func GetTestRepoServers() []*client.Server {
	host := os.Getenv("HYDRAIDE_TEST_HOST")
	if host == "" {
		host = "localhost:50051" // Safe default for local dev
	}
	
	return []*client.Server{
		{
			Host:          host,
			FromIsland:    0,
			ToIsland:      99,
			CACrtPath:     os.Getenv("HYDRAIDE_TEST_CA_CRT"),
			ClientCrtPath: os.Getenv("HYDRAIDE_TEST_CLIENT_CRT"),
			ClientKeyPath: os.Getenv("HYDRAIDE_TEST_CLIENT_KEY"),
		},
	}
}

// GetTestIslandCount returns total islands for testing
func GetTestIslandCount() uint64 {
	if count := os.Getenv("HYDRAIDE_TEST_ISLAND_COUNT"); count != "" {
		if val, err := strconv.ParseUint(count, 10, 64); err == nil {
			return val
		}
	}
	return 100 // Default
}

// GetTestMaxMessageSize returns max message size for testing
func GetTestMaxMessageSize() int {
	if size := os.Getenv("HYDRAIDE_TEST_MAX_MESSAGE_SIZE"); size != "" {
		if val, err := strconv.Atoi(size); err == nil {
			return val
		}
	}
	return 4194304 // 4MB default
}
```

**Example `.env.test` file:**
```bash
# HydrAIDE Test Instance Configuration
HYDRAIDE_TEST_HOST=localhost:50051
HYDRAIDE_TEST_CA_CRT=/path/to/test/ca.crt
HYDRAIDE_TEST_CLIENT_CRT=/path/to/test/client.crt
HYDRAIDE_TEST_CLIENT_KEY=/path/to/test/client.key
HYDRAIDE_TEST_ISLAND_COUNT=100
HYDRAIDE_TEST_MAX_MESSAGE_SIZE=4194304
```

---

## Running Your Tests

```bash
# Run all tests in the suite
go test -v ./...

# Run specific test suite
go test -v -run TestProductTestSuite

# Run specific test within suite
go test -v -run TestProductTestSuite/TestSaveAndLoad

# Run with coverage
go test -v -cover ./...
```

---

## Conclusion

Testing with a real HydrAIDE instance provides confidence that your code works correctly in production-like conditions. The lightweight nature of HydrAIDE makes this approach practical and efficient. Remember to always clean up your test data to ensure repeatable, reliable tests.

For more examples, check out the complete test files in the SDK repository!
