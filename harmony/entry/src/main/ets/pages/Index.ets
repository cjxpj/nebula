import { webview } from '@kit.ArkWeb';
import { http } from '@kit.NetworkKit';

const HTTP_BASE: string = 'http://127.0.0.1:8080';
const OPUI_URL: string = 'https://nebulaopui.cjxpj.com/';

@Entry
@Component
struct Index {
  controller: webview.WebviewController = new webview.WebviewController();
  @State url: string = HTTP_BASE + '/';
  private pollTimer: number = -1;

  aboutToAppear(): void {
    this.waitForHttpReady();
  }

  aboutToDisappear(): void {
    if (this.pollTimer !== -1) {
      clearTimeout(this.pollTimer);
      this.pollTimer = -1;
    }
  }

  // 等 Go HTTP 服务就绪后再加载，避免服务尚未监听导致 WebView 白屏
  private waitForHttpReady(): void {
    const deadline: number = Date.now() + 10000;
    const tryConnect = (): void => {
      if (Date.now() > deadline) {
        this.loadOpui();
        return;
      }
      const req = http.createHttp();
      req.request(HTTP_BASE + '/', {
        method: http.RequestMethod.GET,
        connectTimeout: 2000,
        readTimeout: 2000,
      }).then(() => {
        req.destroy();
        this.loadOpui();
      }).catch(() => {
        req.destroy();
        this.pollTimer = setTimeout(tryConnect, 1000);
      });
    };
    tryConnect();
  }

  private loadOpui(): void {
    this.url = OPUI_URL;
  }

  build() {
    Column() {
      Web({ src: this.url, controller: this.controller })
        .width('100%')
        .height('100%')
        .javaScriptAccess(true)
        .domStorageAccess(true)
    }
    .width('100%')
    .height('100%')
    .backgroundColor('#1A1A2E')
  }
}
