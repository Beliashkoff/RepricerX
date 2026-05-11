// Package mock — in-memory реализация integration.Marketplace для разработки
// без реальных API-ключей. Активируется флагом MOCK_MARKETPLACES=true; в prod
// запрещён guard'ом в config.Load().
package mock

import "github.com/Beliashkoff/RepricerX/internal/integration"

var wbSeed = []integration.SKU{
	{ExternalSKU: "WB-1001", Name: "Футболка хлопок белая", CurrentPrice: 1290, Currency: "RUB", StockCount: 50},
	{ExternalSKU: "WB-1002", Name: "Кружка керамика 350мл", CurrentPrice: 590, Currency: "RUB", StockCount: 120},
	{ExternalSKU: "WB-1003", Name: "Толстовка флис серая", CurrentPrice: 2490, Currency: "RUB", StockCount: 30},
	{ExternalSKU: "WB-1004", Name: "Носки хлопок 5 пар", CurrentPrice: 790, Currency: "RUB", StockCount: 200},
	{ExternalSKU: "WB-1005", Name: "Рюкзак городской 20л", CurrentPrice: 3490, Currency: "RUB", StockCount: 15},
	{ExternalSKU: "WB-1006", Name: "Бутылка для воды 500мл", CurrentPrice: 690, Currency: "RUB", StockCount: 80},
}

var ozonSeed = []integration.SKU{
	{ExternalSKU: "OZ-2001", Name: "Чехол для iPhone 15", CurrentPrice: 990, Currency: "RUB", StockCount: 100},
	{ExternalSKU: "OZ-2002", Name: "Кабель USB-C 1м", CurrentPrice: 350, Currency: "RUB", StockCount: 300},
	{ExternalSKU: "OZ-2003", Name: "Powerbank 10000 мАч", CurrentPrice: 1890, Currency: "RUB", StockCount: 40},
	{ExternalSKU: "OZ-2004", Name: "Наушники TWS", CurrentPrice: 2790, Currency: "RUB", StockCount: 25},
	{ExternalSKU: "OZ-2005", Name: "Подставка для телефона", CurrentPrice: 490, Currency: "RUB", StockCount: 150},
	{ExternalSKU: "OZ-2006", Name: "Лампа настольная LED", CurrentPrice: 1290, Currency: "RUB", StockCount: 20},
}

func seedFor(marketplace string) []integration.SKU {
	switch marketplace {
	case "wb":
		out := make([]integration.SKU, len(wbSeed))
		copy(out, wbSeed)
		return out
	case "ozon":
		out := make([]integration.SKU, len(ozonSeed))
		copy(out, ozonSeed)
		return out
	default:
		return nil
	}
}
