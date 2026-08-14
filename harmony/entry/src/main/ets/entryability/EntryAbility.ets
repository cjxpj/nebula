import { AbilityConstant, UIAbility, Want } from '@kit.AbilityKit';
import { hilog } from '@kit.PerformanceAnalysisKit';
import { window } from '@kit.ArkUI';
import nebula from 'libentry.so';
import { SystemBridge } from '../common/SystemBridge';

const DOMAIN = 0x0000;
const TAG = 'Nebula';

export default class EntryAbility extends UIAbility {
  onCreate(want: Want, launchParam: AbilityConstant.LaunchParam): void {
    hilog.info(DOMAIN, TAG, 'EntryAbility onCreate');

    // 注入应用沙箱数据目录（必须早于 run，否则 Go 写相对路径失败会退出）
    try {
      nebula.setDataDir(this.context.filesDir);
      hilog.info(DOMAIN, TAG, '数据目录已注入: %{public}s', this.context.filesDir);
    } catch (e) {
      hilog.error(DOMAIN, TAG, '注入数据目录失败: %{public}s', JSON.stringify(e));
    }

    // 启动 Go 原生 HTTP 服务（非阻塞，内部会起 goroutine）
    try {
      nebula.run();
      hilog.info(DOMAIN, TAG, 'Go 服务已启动');
    } catch (e) {
      hilog.error(DOMAIN, TAG, 'Go 服务启动失败: %{public}s', JSON.stringify(e));
    }

    // 采集设备信息/电量并推送 Go，订阅电量变化
    SystemBridge.collectDeviceInfo();
    SystemBridge.pushBattery();
    SystemBridge.startBatteryMonitor();

    // 请求通知授权 + 轮询 Go 侧通知
    SystemBridge.requestNotification(this.context);
    SystemBridge.startNotificationPoller();
  }

  onWindowStageCreate(windowStage: window.WindowStage): void {
    windowStage.loadContent('pages/Index', (err) => {
      if (err.code) {
        hilog.error(DOMAIN, TAG, '加载页面失败: %{public}s', JSON.stringify(err));
        return;
      }
      hilog.info(DOMAIN, TAG, '页面加载成功');
    });
  }

  onDestroy(): void {
    hilog.info(DOMAIN, TAG, 'EntryAbility onDestroy');
    SystemBridge.stopNotificationPoller();
    SystemBridge.stopBatteryMonitor();
  }
}
