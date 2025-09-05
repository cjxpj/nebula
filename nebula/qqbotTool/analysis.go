package qqbottool

func Op(o int) string {
	switch o {
	case 0:
		return "服务端进行消息推送"
	case 1:
		return "客户端或服务端发送心跳"
	case 2:
		return "客户端发送鉴权"
	case 6:
		return "客户端恢复连接"
	case 7:
		return "服务端通知客户端重新连接"
	case 9:
		return "当 identify 或 resume 的时候，如果参数有错，服务端会返回该消息"
	case 10:
		return "当客户端与网关建立 ws 连接之后，网关下发的第一条消息"
	case 11:
		return "当发送心跳成功之后，就会收到该消息"
	case 12:
		return "仅用于 http 回调模式的回包，代表机器人收到了平台推送的数据"
	case 13:
		return "开放平台对机器人服务端进行验证"
	default:
		return "未知操作码"
	}
}
