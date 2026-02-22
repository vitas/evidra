export type SubjectDisplay = {
  title: string;
  subtitle: string;
};

const IN_CLUSTER_URL = "https://kubernetes.default.svc";

export function formatSubjectDisplay(rawSubject: string): SubjectDisplay {
  const parts = rawSubject.split(":");
  if (parts.length < 3) {
    return { title: rawSubject, subtitle: "" };
  }

  const app = (parts[0] || "").trim();
  const namespace = (parts[1] || "").trim();
  const clusterRaw = parts.slice(2).join(":").trim();
  if (!app || !namespace || !clusterRaw) {
    return { title: rawSubject, subtitle: "" };
  }

  return {
    title: app,
    subtitle: `${namespace} · ${normalizeCluster(clusterRaw)}`,
  };
}

function normalizeCluster(value: string): string {
  if (value === IN_CLUSTER_URL) return "in-cluster";
  if (value.startsWith("http://") || value.startsWith("https://")) {
    try {
      return new URL(value).host || stripProtocol(value);
    } catch {
      return stripProtocol(value);
    }
  }
  return value;
}

function stripProtocol(value: string): string {
  return value.replace(/^https?:\/\//, "");
}

