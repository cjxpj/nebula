package feishubot

import (
	"strings"

	"github.com/cjxpj/nebula/dto"
)

func init() {
	dto.BotFuncsRegistry["FeiShu"] = Funcs
}

// 机器人函数
var Funcs = map[string]dto.DicFunc{
	"图片": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			list, err := GetImageMsg(d.Inputs.String(1), d.Inputs.String(2))
			return list, err
		},
	},

	"群单发": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			rMsg := d.Inputs.String(2)
			if rMsg != "" {
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				SendGroupMsg(d.Inputs.String(1), rMsg)
			}
			return "", nil
		},
	},

	"私聊": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			rMsg := d.Inputs.String(2)
			if rMsg != "" {
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				SendPrivateMsg(d.Inputs.String(1), rMsg)
			}
			return "", nil
		},
	},

	"回复": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			rMsg := d.Inputs.String(2)
			if rMsg != "" {
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				ReplyMsg(d.Inputs.String(1), rMsg)
			}
			return "", nil
		},
	},

	"撤回": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			RecallMsg(d.Inputs.String(1))
			return "", nil
		},
	},

	"表情回复": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			data, err := AddReaction(
				d.Inputs.String(1),
				d.Inputs.String(2),
			)
			return data, err
		},
	},

	"上传图片": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			imgData, err := parseImgData(d.Inputs.String(1))
			if err != nil {
				return "", err
			}
			return UploadImage(imgData)
		},
	},

	"发送图片": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			data, err := SendGroupImg(
				d.Inputs.String(1),
				d.Inputs.String(2),
			)
			return data, err
		},
	},

	"私聊图片": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			data, err := SendPrivateImg(
				d.Inputs.String(1),
				d.Inputs.String(2),
			)
			return data, err
		},
	},

	"获取群信息": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			data, err := GetChatInfo(d.Inputs.String(1))
			return data, err
		},
	},

	"获取群成员列表": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			data, err := GetChatMemberList(d.Inputs.String(1))
			return data, err
		},
	},
}
