export type ProfilePayload = {
  ok: boolean;
  user: { id: number; email?: string; role: string; status: string };
  hasAccess: boolean;
  allowMessageAnonymization: boolean;
  activeTo?: string;
  maxLinked?: boolean;
  maxBonusGranted?: boolean;
  maxBotLink?: string;
  maxAlsoLinkedTo?: string[];
  telegramLinked?: boolean;
  telegramBotLink?: string;
  activeModes?: { modeId: number; modeName: string; activeTo?: string }[];
  stats?: {
    dialogs: number;
    messages: number;
    liveMessages: number;
    tokens: number;
  };
  billing?: {
    subscription?: {
      id?: number;
      status?: string;
      activeTo?: string;
      tariffName?: string;
      isRecurring?: boolean;
      autoRenewEnabled?: boolean;
      nextRetryAt?: string;
      graceUntil?: string;
      consecutiveFailures?: number;
      cancelReason?: string;
      lastError?: string;
      paymentMethodId?: string;
      paymentMethodType?: string;
      paymentMethodStatus?: string;
    };
    payments?: {
      id: number;
      paymentId: string;
      status: string;
      tariffName: string;
      amount: string;
      currency: string;
      subscriptionMonths: number;
      isRecurring: boolean;
      autoRenewRequested: boolean;
      renewalAttempt: number;
      createdAt: string;
      paidAt?: string;
      cancellationReason?: string;
    }[];
  };
  notifications?: {
    contacts?: { channel: string; address: string; verified: boolean; source: string }[];
    consents?: { channel: string; consentType: string; status: string; reason?: string }[];
    inbox?: {
      id: number;
      templateKey?: string;
      consentType: string;
      title: string;
      body: string;
      status: string;
      createdAt: string;
    }[];
    reachability?: {
      emailAvailable: boolean;
      phoneVerified: boolean;
      pushEnabled: boolean;
      serviceChannels: string[];
      marketingChannels: string[];
      bestChannel: string;
    };
  };
};
