package qqbot_msg

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ============= 自定义菜单 ============

// GetMenu 查询全局自定义菜单（GET /v2/menu）
func (b *QQBot) GetMenu() (*MenuResponse, error) {
	var resp MenuResponse
	if err := b.Get("/v2/menu", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SetMenu 修改全局自定义菜单（PUT /v2/menu）
func (b *QQBot) SetMenu(menu *Menu) (*MenuResponse, error) {
	req := &SetMenuRequest{Menu: menu}
	var resp MenuResponse
	if err := b.Put("/v2/menu", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ============= 指令面板 ============

// GetPanels 查询指令面板列表（GET /v2/panels），scope 必填，cursor 分页游标，limit 每页条数（默认 20，最大 50）
func (b *QQBot) GetPanels(scope, cursor string, limit int) (*PanelListResponse, error) {
	if scope == "" {
		return nil, fmt.Errorf("scope不能为空")
	}
	url := "/v2/panels?scope=" + scope
	if cursor != "" {
		url += "&cursor=" + cursor
	}
	if limit > 0 {
		url += fmt.Sprintf("&limit=%d", limit)
	}
	var resp PanelListResponse
	if err := b.Get(url, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreatePanel 创建指令面板（POST /v2/panels）
func (b *QQBot) CreatePanel(req *CreatePanelRequest) (*CreatePanelResponse, error) {
	if req == nil || req.Scope == "" {
		return nil, fmt.Errorf("scope不能为空")
	}
	var resp CreatePanelResponse
	if err := b.Send("/v2/panels", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetPanel 查询指令面板详情（GET /v2/panels/{panel_id}）
func (b *QQBot) GetPanel(panelID string) (*PanelDetailResponse, error) {
	if panelID == "" {
		return nil, fmt.Errorf("panelID不能为空")
	}
	var resp PanelDetailResponse
	if err := b.Get("/v2/panels/"+panelID, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UpdatePanel 修改指令面板（PUT /v2/panels/{panel_id}）
func (b *QQBot) UpdatePanel(panelID string, req *UpdatePanelRequest) (*UpdatePanelResponse, error) {
	if panelID == "" {
		return nil, fmt.Errorf("panelID不能为空")
	}
	var resp UpdatePanelResponse
	if err := b.Put("/v2/panels/"+panelID, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeletePanel 删除指令面板（DELETE /v2/panels/{panel_id}）
func (b *QQBot) DeletePanel(panelID string) error {
	if panelID == "" {
		return fmt.Errorf("panelID不能为空")
	}
	return b.Delete("/v2/panels/"+panelID, nil)
}

// UpdatePanelTarget 修改指令面板关联对象（PUT /v2/panels/{panel_id}/target）
func (b *QQBot) UpdatePanelTarget(panelID string, req *UpdatePanelTargetRequest) error {
	if panelID == "" {
		return fmt.Errorf("panelID不能为空")
	}
	return b.Put("/v2/panels/"+panelID+"/target", req, nil)
}

// ============= 解析辅助 ============

// ParseMenuJSON 解析菜单 JSON，兼容 items 数组 `[...]` 与菜单对象 `{"items":[...]}`
func ParseMenuJSON(jsonStr string) (*Menu, error) {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return nil, fmt.Errorf("菜单JSON为空")
	}
	if strings.HasPrefix(jsonStr, "[") {
		var items []*MenuItem
		if err := json.Unmarshal([]byte(jsonStr), &items); err != nil {
			return nil, err
		}
		return &Menu{Items: items}, nil
	}
	var menu Menu
	if err := json.Unmarshal([]byte(jsonStr), &menu); err != nil {
		return nil, err
	}
	return &menu, nil
}

// ParsePanelJSON 解析面板 JSON，兼容 items 数组 `[...]` 与面板对象 `{"items":[...]}`
func ParsePanelJSON(jsonStr string) (*Panel, error) {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return nil, fmt.Errorf("面板JSON为空")
	}
	if strings.HasPrefix(jsonStr, "[") {
		var items []*PanelItem
		if err := json.Unmarshal([]byte(jsonStr), &items); err != nil {
			return nil, err
		}
		return &Panel{Items: items}, nil
	}
	var panel Panel
	if err := json.Unmarshal([]byte(jsonStr), &panel); err != nil {
		return nil, err
	}
	return &panel, nil
}
