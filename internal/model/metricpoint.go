package model

// MetricPoint is a persisted system metric sample (for time-range queries in dashboards/monitoring trend charts).
// Corresponds to the service.MetricPoint fields; time is indexed to speed up range queries.
type MetricPoint struct {
	ID       uint    `gorm:"primaryKey" json:"id"`
	Time     int64   `gorm:"index;not null" json:"time"` // unix seconds
	CPU      float64 `json:"cpu"`                        // %
	Mem      float64 `json:"mem"`                        // %
	Load1    float64 `json:"load1"`                      // 1-minute load
	NetRecv  float64 `json:"net_recv"`                   // bytes/s
	NetSent  float64 `json:"net_sent"`                   // bytes/s
}
