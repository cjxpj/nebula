import { batteryInfo, deviceInfo, commonEventManager, BusinessError } from '@kit.BasicServicesKit';
import { notificationManager } from '@kit.NotificationKit';
import { hilog } from '@kit.PerformanceAnalysisKit';
import { common } from '@kit.AbilityKit';
import nebula from 'libentry.so';

const DOMAIN = 0x0000;
const TAG = 'Nebula';

class NotifData {
  title: string = '';
  content: string = '';
}

class DeviceInfoData {
  deviceType: string = '';
  manufacture: string = '';
  brand: string = '';
  marketName: string = '';
  productModel: string = '';
  softwareModel: string = '';
  hardwareModel: string = '';
  osFullName: string = '';
  displayVersion: string = '';
  sdkApiVersion: number = 0;
  abiList: string = '';
}

export class SystemBridge {
  private static subscriber: commonEventManager.CommonEventSubscriber | null = null;
  private static notifPollerId: number = -1;

  // 采集设备信息并推送给 Go 层
  static collectDeviceInfo(): void {
    const info = new DeviceInfoData();
    info.deviceType = deviceInfo.deviceType;
    info.manufacture = deviceInfo.manufacture;
    info.brand = deviceInfo.brand;
    info.marketName = deviceInfo.marketName;
    info.productModel = deviceInfo.productModel;
    info.softwareModel = deviceInfo.softwareModel;
    info.hardwareModel = deviceInfo.hardwareModel;
    info.osFullName = deviceInfo.osFullName;
    info.displayVersion = deviceInfo.displayVersion;
    info.sdkApiVersion = deviceInfo.sdkApiVersion;
    info.abiList = deviceInfo.abiList;
    nebula.setDeviceInfo(JSON.stringify(info));
  }

  // 推送当前电量给 Go 层
  static pushBattery(): void {
    const level: number = batteryInfo.batterySOC;
    const charging: boolean = batteryInfo.chargingStatus === batteryInfo.BatteryChargeState.ENABLE
      || batteryInfo.chargingStatus === batteryInfo.BatteryChargeState.FULL;
    nebula.updateBattery(level, charging);
  }

  // 订阅电量变化事件
  static startBatteryMonitor(): void {
    const subscribeInfo: commonEventManager.CommonEventSubscribeInfo = {
      events: ['usual.event.BATTERY_CHANGED']
    };
    commonEventManager.createSubscriber(subscribeInfo,
      (err: BusinessError, data: commonEventManager.CommonEventSubscriber) => {
        if (err) {
          hilog.error(DOMAIN, TAG, '创建电量订阅者失败: %{public}s', JSON.stringify(err));
          return;
        }
        SystemBridge.subscriber = data;
        commonEventManager.subscribe(data, (err2: BusinessError) => {
          if (err2) {
            hilog.error(DOMAIN, TAG, '订阅电量事件失败: %{public}s', JSON.stringify(err2));
            return;
          }
          SystemBridge.pushBattery();
        });
      });
  }

  // 请求通知授权
  static async requestNotification(context: common.UIAbilityContext): Promise<void> {
    try {
      const enabled: boolean = await notificationManager.isNotificationEnabled();
      if (!enabled) {
        await notificationManager.requestEnableNotification(context);
      }
    } catch (e) {
      hilog.error(DOMAIN, TAG, '请求通知权限失败: %{public}s', JSON.stringify(e));
    }
  }

  // 轮询 Go 侧待发送通知并发布
  static pollAndPublishNotification(): void {
    const json: string = nebula.pollNotification();
    if (json.length === 0) {
      return;
    }
    try {
      const obj: NotifData = JSON.parse(json) as NotifData;
      const request: notificationManager.NotificationRequest = {
        id: Date.now(),
        content: {
          notificationContentType: notificationManager.ContentType.NOTIFICATION_CONTENT_BASIC_TEXT,
          normal: {
            title: obj.title,
            text: obj.content,
          }
        }
      };
      notificationManager.publish(request).then(() => {
        hilog.info(DOMAIN, TAG, '通知已发布');
      }).catch((err: BusinessError) => {
        hilog.error(DOMAIN, TAG, '发布通知失败: %{public}s', JSON.stringify(err));
      });
    } catch (e) {
      hilog.error(DOMAIN, TAG, '解析通知失败: %{public}s', JSON.stringify(e));
    }
  }

  // 启动通知轮询（每 1 秒检查一次 Go 侧通知队列）
  static startNotificationPoller(): void {
    if (SystemBridge.notifPollerId !== -1) {
      return;
    }
    SystemBridge.notifPollerId = setInterval(() => {
      SystemBridge.pollAndPublishNotification();
    }, 1000);
  }

  // 停止通知轮询
  static stopNotificationPoller(): void {
    if (SystemBridge.notifPollerId !== -1) {
      clearInterval(SystemBridge.notifPollerId);
      SystemBridge.notifPollerId = -1;
    }
  }

  // 停止电量监听并取消订阅
  static stopBatteryMonitor(): void {
    if (SystemBridge.subscriber !== null) {
      commonEventManager.unsubscribe(SystemBridge.subscriber, (err: BusinessError) => {
        if (err) {
          hilog.error(DOMAIN, TAG, '取消订阅电量事件失败: %{public}s', JSON.stringify(err));
        }
      });
      SystemBridge.subscriber = null;
    }
  }
}
