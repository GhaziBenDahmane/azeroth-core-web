package web

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
}
type product struct {
	ID            uint32       `json:"id"`
	ItemID        uint32       `json:"itemId"`
	Quantity      uint32       `json:"quantity"`
	Price         uint32       `json:"price"`
	Name          string       `json:"name"`
	Description   string       `json:"description"`
	Category      string       `json:"category"`
	ImageURL      string       `json:"imageUrl"`
	ClassID       uint8        `json:"classId,omitempty"`
	ClassName     string       `json:"className,omitempty"`
	Tier          string       `json:"tier,omitempty"`
	ServiceLevel  uint8        `json:"serviceLevel,omitempty"`
	ServiceAction string       `json:"serviceAction,omitempty"`
	Includes      []string     `json:"includes,omitempty"`
	Items         []bundleItem `json:"items,omitempty"`
	Gold          uint32       `json:"gold,omitempty"`
}

type bundleItem struct {
	ItemID   uint32 `json:"itemId"`
	Quantity uint32 `json:"quantity"`
}
