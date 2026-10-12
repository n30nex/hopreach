package config

import "testing"

func TestMeshMapperRequiresRegionalFenceAndGlobalQuotaInterval(t *testing.T) {
	c := regionalConfig()
	c.MeshMapper.Enabled = true
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.MeshMapper.IntervalHours = .25
	if c.Validate() == nil {
		t.Fatal("global feed polling allowed more than twice daily")
	}
	c.MeshMapper.IntervalHours = 12
	c.Regions.Entries = nil
	if c.Validate() == nil {
		t.Fatal("global import enabled without Canadian IATA allowlist")
	}
}
