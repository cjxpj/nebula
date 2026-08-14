export const run: () => void;
export const setDataDir: (dir: string) => void;
export const getOpuiUrl: () => string;
export const setDeviceInfo: (info: string) => void;
export const updateBattery: (level: number, charging: boolean) => void;
export const pollNotification: () => string;
