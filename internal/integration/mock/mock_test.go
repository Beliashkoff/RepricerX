package mock

import (
	"context"
	"errors"
	"testing"

	"github.com/Beliashkoff/RepricerX/internal/integration"
)

func TestStore_LazySeedListWB(t *testing.T) {
	s := NewStore()
	got := s.List("wb", "shop-1")
	if len(got) != len(wbSeed) {
		t.Fatalf("ожидалось %d SKU, получено %d", len(wbSeed), len(got))
	}
	// порядок — отсортированный по ExternalSKU
	for i := 1; i < len(got); i++ {
		if got[i-1].ExternalSKU > got[i].ExternalSKU {
			t.Fatalf("List вернул неотсортированный список: %s > %s", got[i-1].ExternalSKU, got[i].ExternalSKU)
		}
	}
}

func TestStore_LazySeedListOzon(t *testing.T) {
	s := NewStore()
	got := s.List("ozon", "shop-1")
	if len(got) != len(ozonSeed) {
		t.Fatalf("ожидалось %d SKU, получено %d", len(ozonSeed), len(got))
	}
}

func TestStore_PartitionedByShop(t *testing.T) {
	s := NewStore()
	first := s.List("wb", "shop-A")
	_ = s.Apply("wb", "shop-A", []integration.PriceUpdate{
		{ExternalSKU: first[0].ExternalSKU, NewPrice: 9999},
	})

	other := s.List("wb", "shop-B")
	for _, sku := range other {
		if sku.ExternalSKU == first[0].ExternalSKU && sku.CurrentPrice == 9999 {
			t.Fatal("Apply в shop-A не должен затрагивать shop-B")
		}
	}
}

func TestStore_PartitionedByMarketplace(t *testing.T) {
	s := NewStore()
	wb := s.List("wb", "shop-1")
	if len(wb) == 0 {
		t.Fatal("пустой WB seed")
	}
	// После перехода WB-адаптера на nmID-as-ExternalSKU цифровой префикс
	// различает маркетплейсы вместо буквенного. WB seed использует "1001XXXXX",
	// vendorCode хранит человеко-читаемый "WB-…".
	if wb[0].VendorCode[:2] != "WB" {
		t.Fatalf("ожидался WB-vendorCode-префикс, получен %q", wb[0].VendorCode)
	}
	oz := s.List("ozon", "shop-1")
	if oz[0].ExternalSKU[:2] != "OZ" {
		t.Fatalf("ожидался OZ-префикс, получен %q", oz[0].ExternalSKU)
	}
}

func TestStore_ApplyMutatesPrice(t *testing.T) {
	s := NewStore()
	before := s.List("wb", "shop-1")
	target := before[0].ExternalSKU

	if err := s.Apply("wb", "shop-1", []integration.PriceUpdate{
		{ExternalSKU: target, NewPrice: 1234.56},
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	after := s.List("wb", "shop-1")
	for _, sku := range after {
		if sku.ExternalSKU == target {
			if sku.CurrentPrice != 1234.56 {
				t.Fatalf("ожидалась цена 1234.56, получено %v", sku.CurrentPrice)
			}
			return
		}
	}
	t.Fatalf("SKU %s исчез после Apply", target)
}

func TestStore_ApplyIgnoresUnknownSKU(t *testing.T) {
	s := NewStore()
	_ = s.List("wb", "shop-1") // seed
	err := s.Apply("wb", "shop-1", []integration.PriceUpdate{
		{ExternalSKU: "WB-DOES-NOT-EXIST", NewPrice: 100},
	})
	if err != nil {
		t.Fatalf("Apply на неизвестном SKU не должен возвращать ошибку, получено: %v", err)
	}
}

func TestAdapter_ImplementsInterface(t *testing.T) {
	var _ integration.Marketplace = (*Adapter)(nil)
}

func TestAdapter_ListThenUpdateThenListRoundTrip(t *testing.T) {
	s := NewStore()
	a := &Adapter{store: s, marketplace: "wb", shopID: "shop-X"}

	skus, err := a.ListSKUs(context.Background())
	if err != nil {
		t.Fatalf("ListSKUs: %v", err)
	}
	if len(skus) == 0 {
		t.Fatal("ожидался непустой ListSKUs после lazy seed")
	}

	target := skus[2].ExternalSKU
	if err := a.UpdatePrices(context.Background(), []integration.PriceUpdate{
		{ExternalSKU: target, NewPrice: 4242},
	}); err != nil {
		t.Fatalf("UpdatePrices: %v", err)
	}

	after, _ := a.ListSKUs(context.Background())
	for _, sku := range after {
		if sku.ExternalSKU == target && sku.CurrentPrice != 4242 {
			t.Fatalf("после UpdatePrices ожидалась цена 4242, получено %v", sku.CurrentPrice)
		}
	}
}

func TestBuilder_HappyPath(t *testing.T) {
	store := NewStore()
	build := NewBuilder(store, "wb")
	client, err := build("shop-1", []byte(`{"api_token":"mock-token"}`))
	if err != nil {
		t.Fatalf("ожидался успех, получено: %v", err)
	}
	if client == nil {
		t.Fatal("client = nil")
	}
	if err := client.TestAuth(context.Background()); err != nil {
		t.Fatalf("TestAuth должен проходить без ошибок, получено: %v", err)
	}
}

func TestBuilder_InvalidMarker(t *testing.T) {
	store := NewStore()
	build := NewBuilder(store, "wb")
	client, err := build("shop-1", []byte(`{"api_token":"invalid"}`))
	if !errors.Is(err, integration.ErrUnauthorized) {
		t.Fatalf("ожидалась ErrUnauthorized, получено: %v", err)
	}
	if client != nil {
		t.Fatalf("при ErrUnauthorized client должен быть nil, получено: %T", client)
	}
}

func TestBuilder_UnknownMarketplaceReturnsEmpty(t *testing.T) {
	store := NewStore()
	build := NewBuilder(store, "yandex_market")
	client, err := build("shop-1", []byte(`{"api_token":"x"}`))
	if err != nil {
		t.Fatalf("неизвестный маркетплейс не должен ронять билдер: %v", err)
	}
	skus, err := client.ListSKUs(context.Background())
	if err != nil {
		t.Fatalf("ListSKUs: %v", err)
	}
	if len(skus) != 0 {
		t.Fatalf("ожидался пустой список для неизвестного маркетплейса, получено %d", len(skus))
	}
}
