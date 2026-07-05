export type NetworkEndpoint = {
  interfaceName: string;
  ipAddress: string;
  displayName: string;
  isPrivateIpv4: boolean;
  isUp: boolean;
};

export type FileSelection = {
  path: string;
  name: string;
  sizeBytes: number;
  modifiedAt: string;
  isSymlink: boolean;
  symlinkWarning?: string;
};

export type TransferDetails = {
  id: string;
  status: string;
  sourceFileName: string;
  sourcePath?: string;
  sourceSizeBytes: number;
  bindIp?: string;
  port?: number;
  maxDownloads: number;
  completedDownloads: number;
  remainingDownloads: number;
  expiresAt: string;
  createdAt: string;
  shareUrl?: string;
  lastErrorMessage?: string;
};

export type IncomingTransfer = {
  sessionId: string;
  status: string;
  fileName?: string;
  sizeBytes?: number;
  bytesReceived: number;
  savedPath?: string;
  errorMessage?: string;
  startedAt: string;
  completedAt?: string;
};

export type Settings = {
  language: "ru" | "en";
  theme: "light" | "dark" | "system";
  defaultRelayUrl: string;
  defaultExpiryMinutes: number;
  defaultMaxDownloads: number;
  defaultCalculateSha: boolean;
  warnTrustedLocalOnly: boolean;
  maxActiveTransfers: number;
  confirmCloseWithLinks: boolean;
  cloudPubToken: string;
};

export type Diagnostics = {
  version: string;
  goVersion: string;
  wailsVersion: string;
  sqlitePath: string;
  logsPath: string;
  activeServerCount: number;
  activeTransferCount: number;
};

export type TransferLimits = {
  localSingleFileLimitLabel: string;
  internetSingleFileLimitGb: number;
  internetSingleFileLimitText: string;
  multiFileSupported: boolean;
  multiFileAdvice: string;
};

export type RelayRecommendation = {
  url: string;
  isLocalLan: boolean;
};

type AppApi = {
  SelectFile(): Promise<FileSelection>;
  SelectFilesAsZip(): Promise<FileSelection>;
  GetAvailableNetworkEndpoints(): Promise<NetworkEndpoint[]>;
  CreateTransfer(request: {
    sourcePath: string;
    bindIp: string;
    expiresInMinutes: number;
    maxDownloads: number;
    calculateHash: boolean;
  }): Promise<TransferDetails>;
  CreateInternetTransfer(request: {
    sourcePath: string;
    relayUrl: string;
    recipientId: string;
    expiresInMinutes: number;
    maxDownloads: number;
    calculateHash: boolean;
  }): Promise<TransferDetails>;
  CreateCloudPubTransfer(request: {
    sourcePath: string;
    bindIp: string;
    expiresInMinutes: number;
    maxDownloads: number;
    calculateHash: boolean;
  }): Promise<TransferDetails>;
  CreateDirectTransfer(request: {
    sourcePath: string;
    brokerUrl: string;
    expiresInMinutes: number;
    maxDownloads: number;
    calculateHash: boolean;
  }): Promise<TransferDetails>;
  AcceptDirectTransfer(ticket: string): Promise<IncomingTransfer>;
  ListIncomingTransfers(): Promise<IncomingTransfer[]>;
  CancelDirectSession(sessionId: string): Promise<void>;
  GetTransferLimits(): Promise<TransferLimits>;
  GetRecommendedRelayURL(): Promise<RelayRecommendation>;
  ListActiveTransfers(): Promise<TransferDetails[]>;
  GetTransfer(id: string): Promise<TransferDetails>;
  CancelTransfer(id: string): Promise<void>;
  GetTransferQRCode(id: string): Promise<{ pngBase64: string }>;
  SaveTransferQRCode(id: string): Promise<{ path: string }>;
  CopyTransferLink(id: string): Promise<void>;
  RevealTransferSourceFile(id: string): Promise<void>;
  ListTransferHistory(query?: unknown): Promise<TransferDetails[]>;
  DeleteHistoryItem(id: string): Promise<void>;
  ClearTransferHistory(): Promise<void>;
  GetSettings(): Promise<Settings>;
  UpdateSettings(request: Settings): Promise<Settings>;
  GetDiagnostics(): Promise<Diagnostics>;
};

declare global {
  interface Window {
    go?: { main?: { App?: AppApi } };
  }
}

function app(): AppApi {
  const api = window.go?.main?.App;
  if (!api) {
    throw new Error("Wails API is not available. Run inside Wails dev/build.");
  }
  return api;
}

export const api = {
  selectFile: () => app().SelectFile(),
  selectFilesAsZip: () => app().SelectFilesAsZip(),
  endpoints: () => app().GetAvailableNetworkEndpoints(),
  createTransfer: (request: Parameters<AppApi["CreateTransfer"]>[0]) => app().CreateTransfer(request),
  createInternetTransfer: (request: Parameters<AppApi["CreateInternetTransfer"]>[0]) => app().CreateInternetTransfer(request),
  createCloudPubTransfer: (request: Parameters<AppApi["CreateCloudPubTransfer"]>[0]) => app().CreateCloudPubTransfer(request),
  createDirectTransfer: (request: Parameters<AppApi["CreateDirectTransfer"]>[0]) => app().CreateDirectTransfer(request),
  acceptDirectTransfer: (ticket: string) => app().AcceptDirectTransfer(ticket),
  incomingTransfers: () => app().ListIncomingTransfers(),
  cancelDirect: (sessionId: string) => app().CancelDirectSession(sessionId),
  limits: () => app().GetTransferLimits(),
  recommendedRelay: () => app().GetRecommendedRelayURL(),
  activeTransfers: () => app().ListActiveTransfers(),
  transfer: (id: string) => app().GetTransfer(id),
  cancel: (id: string) => app().CancelTransfer(id),
  qr: (id: string) => app().GetTransferQRCode(id),
  saveQr: (id: string) => app().SaveTransferQRCode(id),
  copyLink: (id: string) => app().CopyTransferLink(id),
  reveal: (id: string) => app().RevealTransferSourceFile(id),
  history: () => app().ListTransferHistory({}),
  deleteHistory: (id: string) => app().DeleteHistoryItem(id),
  clearHistory: () => app().ClearTransferHistory(),
  settings: () => app().GetSettings(),
  updateSettings: (settings: Settings) => app().UpdateSettings(settings),
  diagnostics: () => app().GetDiagnostics(),
};
