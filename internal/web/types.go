package web

import "time"

type account struct {
	ID       uint32 `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	GMLevel  uint8  `json:"gmLevel,omitempty"`
}
type character struct {
	GUID      uint32 `json:"guid"`
	Name      string `json:"name"`
	Race      uint8  `json:"race"`
	Class     uint8  `json:"class"`
	Gender    uint8  `json:"gender"`
	Level     uint8  `json:"level"`
	Zone      uint16 `json:"zone"`
	Online    bool   `json:"online"`
	TotalTime uint32 `json:"totalTime"`
	Money     uint32 `json:"money,omitempty"`
	Guild     string `json:"guild,omitempty"`
	GuildID   uint32 `json:"guildId,omitempty"`
}

type armoryItem struct {
	Slot          uint8             `json:"slot"`
	Entry         uint32            `json:"entry"`
	Name          string            `json:"name"`
	Quality       uint8             `json:"quality"`
	DisplayID     uint32            `json:"displayId"`
	ItemLevel     uint16            `json:"itemLevel"`
	RequiredLevel uint8             `json:"requiredLevel"`
	Armor         uint32            `json:"armor"`
	InventoryType uint8             `json:"inventoryType"`
	SetID         uint32            `json:"setId,omitempty"`
	Icon          string            `json:"icon"`
	Durability    uint16            `json:"durability,omitempty"`
	MaxDurability uint16            `json:"maxDurability,omitempty"`
	Enchantments  string            `json:"-"`
	Enhancements  []itemEnhancement `json:"enhancements,omitempty"`
	Stats         []struct {
		Type  int16 `json:"type"`
		Value int16 `json:"value"`
	} `json:"stats"`
}

type itemEnhancement struct {
	Slot          uint8  `json:"slot"`
	Kind          string `json:"kind"`
	EnchantmentID uint32 `json:"enchantmentId"`
	ItemID        uint32 `json:"itemId,omitempty"`
	Name          string `json:"name,omitempty"`
}
type product struct {
	ID              uint32           `json:"id"`
	ItemID          uint32           `json:"itemId"`
	Quantity        uint32           `json:"quantity"`
	Price           uint32           `json:"price"`
	Name            string           `json:"name"`
	Description     string           `json:"description"`
	Category        string           `json:"category"`
	ImageURL        string           `json:"imageUrl"`
	ClassID         uint8            `json:"classId,omitempty"`
	ClassName       string           `json:"className,omitempty"`
	Tier            string           `json:"tier,omitempty"`
	ServiceLevel    uint8            `json:"serviceLevel,omitempty"`
	ServiceAction   string           `json:"serviceAction,omitempty"`
	Includes        []string         `json:"includes,omitempty"`
	Items           []bundleItem     `json:"items,omitempty"`
	Gold            uint32           `json:"gold,omitempty"`
	Active          bool             `json:"active"`
	StartsAt        *time.Time       `json:"startsAt,omitempty"`
	EndsAt          *time.Time       `json:"endsAt,omitempty"`
	PerAccountLimit uint32           `json:"perAccountLimit,omitempty"`
	Featured        bool             `json:"featured"`
	SalePrice       uint32           `json:"salePrice,omitempty"`
	StockLimit      uint32           `json:"stockLimit,omitempty"`
	SoldCount       uint32           `json:"soldCount,omitempty"`
	CategoryOrder   int              `json:"categoryOrder,omitempty"`
	Tags            string           `json:"tags,omitempty"`
	Visibility      string           `json:"visibility,omitempty"`
	VariantRequired bool             `json:"variantRequired,omitempty"`
	BundleID        uint64           `json:"bundleId,omitempty"`
	Variants        []productVariant `json:"variants,omitempty"`
	Collections     []string         `json:"collections,omitempty"`
}

type productVariant struct {
	ID              uint64       `json:"id"`
	Name            string       `json:"name"`
	SKU             string       `json:"sku"`
	PriceAdjustment int32        `json:"priceAdjustment"`
	Active          bool         `json:"active"`
	SortOrder       int          `json:"sortOrder"`
	Items           []bundleItem `json:"items,omitempty"`
}

type bundleItem struct {
	ItemID        uint32 `json:"itemId"`
	Quantity      uint32 `json:"quantity"`
	Name          string `json:"name,omitempty"`
	Quality       uint8  `json:"quality,omitempty"`
	ItemLevel     uint16 `json:"itemLevel,omitempty"`
	RequiredLevel uint8  `json:"requiredLevel,omitempty"`
	InventoryType uint8  `json:"inventoryType,omitempty"`
	DisplayID     uint32 `json:"displayId,omitempty"`
}
