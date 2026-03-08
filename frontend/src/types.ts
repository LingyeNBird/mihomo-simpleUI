export interface Subscription {
  id: number;
  name: string;
  url: string;
  enabled: boolean;
  filePath: string;
  lastRefreshedAt?: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface RefreshResult {
  subscription: Subscription;
  warnings?: string[];
}

export interface ProxyGroup {
  name: string;
  type: string;
  current: string;
  all: string[];
}

export interface AppStatus {
  appStartedAt: string;
  configPath: string;
  databasePath: string;
  mihomoControllerUrl: string;
  mihomoConnected: boolean;
  mihomoVersion?: string;
  subscriptionCount: number;
  lastConfigSyncAt?: string;
  lastConfigError?: string;
}

export interface ConfigSyncResult {
  configPath: string;
  reloaded: boolean;
  warnings?: string[];
}

export interface SubscriptionContent {
  id: number;
  name: string;
  filePath: string;
  content: string;
}
