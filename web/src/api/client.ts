// API Client with automatic token refresh

interface LoginResponse {
  access_token: string;
  expires_in: number;
  api_token: string;
}

interface Peer {
  id: number;
  name: string;
  public_key: string;
  assigned_ip: string;
  enabled: boolean;
  created_at: string;
  is_online: boolean;
  latest_handshake: string;
  transfer_rx: number;
  transfer_tx: number;
  endpoint: string;
}

interface ApiError {
  error: string;
  message?: string;
}

class ApiClient {
  private accessToken: string | null = null;
  private tokenExpiry: number = 0;
  private static TOKEN_KEY = 'wg_access_token';
  private static EXPIRY_KEY = 'wg_token_expiry';

  constructor() {
    // Load token from localStorage on init
    this.loadStoredToken();
  }

  private loadStoredToken() {
    try {
      const token = localStorage.getItem(ApiClient.TOKEN_KEY);
      const expiry = localStorage.getItem(ApiClient.EXPIRY_KEY);
      if (token && expiry) {
        const expiryTime = parseInt(expiry, 10);
        if (Date.now() < expiryTime) {
          this.accessToken = token;
          this.tokenExpiry = expiryTime;
        } else {
          // Token expired, clear it
          this.clearToken();
        }
      }
    } catch {
      // localStorage not available
    }
  }

  setToken(token: string, expiresIn: number) {
    this.accessToken = token;
    this.tokenExpiry = Date.now() + (expiresIn * 1000) - 30000; // 30s buffer
    // Persist to localStorage
    try {
      localStorage.setItem(ApiClient.TOKEN_KEY, token);
      localStorage.setItem(ApiClient.EXPIRY_KEY, this.tokenExpiry.toString());
    } catch {
      // localStorage not available
    }
  }

  clearToken() {
    this.accessToken = null;
    this.tokenExpiry = 0;
    try {
      localStorage.removeItem(ApiClient.TOKEN_KEY);
      localStorage.removeItem(ApiClient.EXPIRY_KEY);
    } catch {
      // localStorage not available
    }
  }

  isAuthenticated(): boolean {
    return this.accessToken !== null && Date.now() < this.tokenExpiry;
  }

  getToken(): string | null {
    return this.accessToken;
  }

  private async request<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(options.headers as Record<string, string>),
    };

    if (this.accessToken) {
      headers['Authorization'] = `Bearer ${this.accessToken}`;
    }

    const response = await fetch(`/api/v1${endpoint}`, {
      ...options,
      headers,
      credentials: 'include',
    });

    if (response.status === 401 && endpoint !== '/auth/login') {
      // Try to refresh token
      const refreshed = await this.refresh();
      if (refreshed) {
        headers['Authorization'] = `Bearer ${this.accessToken}`;
        const retryResponse = await fetch(`/api/v1${endpoint}`, {
          ...options,
          headers,
          credentials: 'include',
        });
        if (!retryResponse.ok) {
          const error: ApiError = await retryResponse.json();
          throw new Error(error.error || 'Request failed');
        }
        return retryResponse.json();
      } else {
        this.clearToken();
        window.location.href = '/login';
        throw new Error('Session expired');
      }
    }

    if (!response.ok) {
      const error: ApiError = await response.json();
      throw new Error(error.error || 'Request failed');
    }

    // Handle empty responses
    const text = await response.text();
    return text ? JSON.parse(text) : ({} as T);
  }

  async login(username: string, password: string): Promise<boolean> {
    try {
      const data = await this.request<LoginResponse>('/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username, password }),
      });
      this.setToken(data.access_token, data.expires_in);
      return true;
    } catch {
      return false;
    }
  }

  async refresh(): Promise<boolean> {
    try {
      const response = await fetch('/api/v1/auth/refresh', {
        method: 'POST',
        credentials: 'include',
      });
      if (!response.ok) return false;
      const data: LoginResponse = await response.json();
      this.setToken(data.access_token, data.expires_in);
      return true;
    } catch {
      return false;
    }
  }

  async logout(): Promise<void> {
    try {
      await this.request('/auth/logout', { method: 'POST' });
    } finally {
      this.clearToken();
    }
  }

  async getPeers(): Promise<Peer[]> {
    const peers = await this.request<Peer[] | null>('/peers');
    return peers || [];
  }

  async createPeer(name: string): Promise<Peer> {
    return this.request<Peer>('/peers', {
      method: 'POST',
      body: JSON.stringify({ name }),
    });
  }

  async deletePeer(ip: string): Promise<void> {
    await this.request(`/peers/${ip}`, { method: 'DELETE' });
  }

  async updatePeer(ip: string, data: { name?: string; enabled?: boolean }): Promise<Peer> {
    return this.request<Peer>(`/peers/${ip}`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    });
  }

  async downloadConfig(ip: string, filename: string): Promise<void> {
    const response = await fetch(`/api/v1/peers/${ip}/config`, {
      headers: {
        'Authorization': `Bearer ${this.accessToken}`,
      },
      credentials: 'include',
    });

    if (!response.ok) {
      throw new Error('Failed to download config');
    }

    const blob = await response.blob();
    const url = window.URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    window.URL.revokeObjectURL(url);
    document.body.removeChild(a);
  }

  async getQRCode(ip: string): Promise<string> {
    const response = await fetch(`/api/v1/peers/${ip}/qrcode`, {
      headers: {
        'Authorization': `Bearer ${this.accessToken}`,
      },
      credentials: 'include',
    });

    if (!response.ok) {
      throw new Error('Failed to get QR code');
    }

    const blob = await response.blob();
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onloadend = () => resolve(reader.result as string);
      reader.onerror = reject;
      reader.readAsDataURL(blob);
    });
  }

  async getSettings(): Promise<Settings> {
    return this.request<Settings>('/settings');
  }

  async updateSettings(data: UpdateSettingsRequest): Promise<void> {
    await this.request('/settings', {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  async getPeerLogs(ip: string): Promise<ConnectionLog[]> {
    const logs = await this.request<ConnectionLog[] | null>(`/peers/${ip}/logs`);
    return logs || [];
  }

  // Tailscale methods
  async getTailscaleStatus(): Promise<TailscaleStatus> {
    return this.request<TailscaleStatus>('/tailscale/status');
  }

  async connectTailscale(): Promise<TailscaleConnectResponse> {
    return this.request<TailscaleConnectResponse>('/tailscale/connect', {
      method: 'POST',
    });
  }

  async disconnectTailscale(): Promise<TailscaleResponse> {
    return this.request<TailscaleResponse>('/tailscale/disconnect', {
      method: 'POST',
    });
  }

  async enableTailscaleRouting(): Promise<TailscaleResponse> {
    return this.request<TailscaleResponse>('/tailscale/routing/enable', {
      method: 'POST',
    });
  }

  async disableTailscaleRouting(): Promise<TailscaleResponse> {
    return this.request<TailscaleResponse>('/tailscale/routing/disable', {
      method: 'POST',
    });
  }

  async getTailscaleRoutes(): Promise<TailscaleRoutes> {
    return this.request<TailscaleRoutes>('/tailscale/routes');
  }
}

interface TailscalePeer {
  name: string;
  hostname: string;
  tailscale_ip: string;
  allowed_ips: string[];
  primary_routes?: string[];
  online: boolean;
}

interface TailscaleRoute {
  subnet: string;
  peer_name: string;
}

interface TailscaleStatus {
  installed: boolean;
  connected: boolean;
  backend_state: string;
  auth_url?: string;
  self?: TailscalePeer;
  peers: TailscalePeer[];
  routes: TailscaleRoute[];
  routing_enabled: boolean;
}

interface TailscaleConnectResponse {
  success: boolean;
  connected: boolean;
  backend_state: string;
  auth_url?: string;
  message: string;
}

interface TailscaleResponse {
  success: boolean;
  message: string;
  routes?: TailscaleRoute[];
}

interface TailscaleRoutes {
  routes: TailscaleRoute[];
  peers: TailscalePeer[];
}

interface Settings {
  dns: string;
  allowed_ips: string;
  logging_enabled: boolean;
  api_token: string;
}

interface UpdateSettingsRequest {
  dns?: string;
  allowed_ips?: string;
  logging_enabled?: boolean;
  admin_password?: string;
}

interface ConnectionLog {
  id: number;
  peer_id: number;
  endpoint: string;
  connected_at: string;
}

export const api = new ApiClient();
export type { Peer, ApiError, Settings, ConnectionLog, TailscaleStatus, TailscalePeer, TailscaleRoute, TailscaleConnectResponse, TailscaleResponse };
