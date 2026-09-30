// Links on the admin "System" tab: service dashboards (monitoring, queues)
// and manual operations (for example, starting a workflow). Every
// installation has its own: put your dashboards here. An empty list hides
// the block in the admin panel.

export type OpsLink = { label: string; href: string; title?: string };

export const SYSTEM_LINKS: readonly OpsLink[] = [];

export const SYSTEM_ACTIONS: readonly OpsLink[] = [];
