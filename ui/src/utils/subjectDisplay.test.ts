import { describe, expect, it } from "vitest";

import { formatSubjectDisplay } from "./subjectDisplay";

describe("formatSubjectDisplay", () => {
  it("formats in-cluster subject", () => {
    expect(formatSubjectDisplay("guestbook-demo:demo:https://kubernetes.default.svc")).toEqual({
      title: "guestbook-demo",
      subtitle: "demo · in-cluster",
    });
  });

  it("keeps non-url cluster values", () => {
    expect(formatSubjectDisplay("payments-api:prod-eu:eu-1")).toEqual({
      title: "payments-api",
      subtitle: "prod-eu · eu-1",
    });
  });

  it("strips protocol from non in-cluster url", () => {
    expect(formatSubjectDisplay("svc:prod:https://k8s.example.internal:6443")).toEqual({
      title: "svc",
      subtitle: "prod · k8s.example.internal:6443",
    });
  });

  it("falls back on invalid format", () => {
    expect(formatSubjectDisplay("broken-subject")).toEqual({
      title: "broken-subject",
      subtitle: "",
    });
  });
});

