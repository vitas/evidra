export type SubjectInfo = {
  subject: string;
  namespace: string;
  cluster: string;
};

export type EventItem = {
  id: string;
  source: string;
  type: string;
  time: string;
  subject: string;
  extensions?: Record<string, unknown>;
  data?: unknown;
  integrity_hash?: string;
};

export type ChangeItem = {
  id: string;
  change_id?: string;
  permalink?: string;
  subject: string;
  application?: string;
  project?: string;
  target_cluster?: string;
  namespace?: string;
  primary_provider: string;
  primary_reference?: string;
  revision?: string;
  initiator?: string;
  external_change_id?: string;
  ticket_id?: string;
  approval_reference?: string;
  has_approvals?: boolean;
  result_status: "succeeded" | "failed" | "unknown";
  health_status: "healthy" | "degraded" | "progressing" | "missing" | "unknown";
  health_at_operation_start?: "healthy" | "degraded" | "progressing" | "missing" | "unknown" | string;
  health_after_deploy?: "healthy" | "degraded" | "progressing" | "missing" | "unknown" | string;
  post_deploy_degradation?: {
    observed: boolean;
    first_timestamp?: string;
  };
  evidence_last_updated_at?: string;
  evidence_window_seconds?: number;
  evidence_may_be_incomplete?: boolean;
  started_at: string;
  completed_at: string;
  event_count: number;
};

export type ChangeDetail = ChangeItem & {
  events?: EventItem[];
};

export type ChangeTimeline = {
  items: EventItem[];
};

export type ChangeEvidence = {
  change: ChangeItem;
  supporting_observations: EventItem[];
  approvals?: Array<{
    source?: string;
    identity?: string;
    timestamp?: string;
    reference?: string;
    summary?: string;
  }>;
};

export type ExportJob = {
  id: string;
  status: "pending" | "completed" | "failed" | string;
  artifact_uri?: string;
  error?: string;
};
