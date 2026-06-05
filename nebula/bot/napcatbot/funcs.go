package napcatbot

import (
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/dto"
)

// 机器人函数
var Funcs = map[string]dto.DicFunc{
	"群列表": {
		L: "0",
		Fn: func(d *dto.DicInputs) (any, error) {
			list, err := GetGroupList()
			return string(list), err
		},
	},

	"禁言": {
		L: "3",
		Fn: func(d *dto.DicInputs) (any, error) {
			GroupMute(d.Inputs.Int64(1), d.Inputs.Int64(2), d.Inputs.Int64(3))
			return "", nil
		},
	},

	"点赞": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			Like(d.Inputs.Int64(1), d.Inputs.Int64(2))
			return "", nil
		},
	},

	"戳一戳": {
		L: "1|2",
		Fn: func(d *dto.DicInputs) (any, error) {
			if d.Inputs.LenOk("2") {
				Nudge(d.Inputs.Int64(1), d.Inputs.Int64(2))
			} else {
				NudgePrivate(d.Inputs.Int64(1))
			}
			return "", nil
		},
	},

	"撤回": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			Recall(d.Inputs.Int64(1))
			return "", nil
		},
	},

	"全体禁言": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			GroupMuteAll(d.Inputs.Int64(1), true)
			return "", nil
		},
	},

	"全体解禁": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			GroupMuteAll(d.Inputs.Int64(1), false)
			return "", nil
		},
	},

	"设置群头衔": {
		L: "3",
		Fn: func(d *dto.DicInputs) (any, error) {
			SetGroupSpecialTitle(
				d.Inputs.Int64(1),
				d.Inputs.Int64(2),
				d.Inputs.String(3),
			)
			return "", nil
		},
	},

	"设置群管理": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			SetGroupAdmin(d.Inputs.Int64(1), d.Inputs.Int64(2), true)
			return "", nil
		},
	},

	"取消群管理": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			SetGroupAdmin(d.Inputs.Int64(1), d.Inputs.Int64(2), false)
			return "", nil
		},
	},

	"获取群信息": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			data, err := GetGroupInfo(d.Inputs.Int64(1))
			return string(data), err
		},
	},

	"获取群成员信息": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			data, err := GetGroupMemberInfo(
				d.Inputs.Int64(1),
				d.Inputs.Int64(2),
			)
			return string(data), err
		},
	},

	"获取群成员列表": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			data, err := GetGroupMemberList(d.Inputs.Int64(1), false)
			return string(data), err
		},
	},

	"踢": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			GroupKick(d.Inputs.Int64(1), d.Inputs.Int64(2), false)
			return "", nil
		},
	},

	"设置群名": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			SetGroupName(d.Inputs.Int64(1), d.Inputs.String(2))
			return "", nil
		},
	},

	"设置群成员名字": {
		L: "3",
		Fn: func(d *dto.DicInputs) (any, error) {
			SetGroupMemberCard(
				d.Inputs.Int64(1),
				d.Inputs.Int64(2),
				d.Inputs.String(3),
			)
			return "", nil
		},
	},

	"获取消息详情": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			data, err := GetMsg(d.Inputs.Int64(1))
			return string(data), err
		},
	},

	"获取好友列表": {
		L: "0",
		Fn: func(d *dto.DicInputs) (any, error) {
			data, err := GetFriendList()
			return string(data), err
		},
	},

	"群单发": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			rMsg := d.Inputs.String(2)
			if rMsg != "" {
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				SendGroupText(d.Inputs.Int64(1), rMsg)
			}
			return "", nil
		},
	},

	"发送视频": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			SendGroupVideo(d.Inputs.Int64(1), d.Inputs.String(2))
			return "", nil
		},
	},

	"发送语音": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			SendGroupRecord(d.Inputs.Int64(1), d.Inputs.String(2))
			return "", nil
		},
	},

	"发送音乐卡片": {
		L: "5",
		Fn: func(d *dto.DicInputs) (any, error) {
			SendGroupMusic(
				d.Inputs.Int64(1),
				d.Inputs.String(2),
				d.Inputs.String(3),
				d.Inputs.String(4),
				d.Inputs.String(5),
			)
			return "", nil
		},
	},

	"私聊": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			rMsg := d.Inputs.String(2)
			if rMsg != "" {
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				SendPrivateText(d.Inputs.Int64(1), rMsg)
			}
			return "", nil
		},
	},

	"构造聊天记录": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			id := MultiMsgPut(
				d.Inputs.String(1),
				d.Inputs.String(2),
			)
			return strconv.FormatInt(id, 10), nil
		},
	},

	"发送聊天记录": {
		L: "3..",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.Int64(1)
			timestamp := d.Inputs.Int64(2)
			n := d.Inputs.Len()
			ids := make([]int64, 0, n-2)
			for i := 3; i <= n; i++ {
				ids = append(ids, d.Inputs.Int64(i))
			}
			res, err := MultiMsg(groupId, timestamp, ids)
			return res, err
		},
	},
}
