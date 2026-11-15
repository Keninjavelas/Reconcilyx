import test from "node:test";
import assert from "node:assert/strict";
import {
  EVIDENCE_BANNER_TEXT,
  EVIDENCE_RUN_ID,
  EVIDENCE_VERIFIED_RUNS,
  REPLAY_AUDIT,
  REPLAY_CLUSTERS,
  REPLAY_HEALTH,
  REPLAY_POLICIES,
  REPLAY_PROPOSALS,
  REPLAY_SAVINGS,
  computeReplayWhatIf,
  getReplayProposalDetail,
  replayMutationBlockedError,
} from "../src/lib/evidenceReplay.ts";

test("evidence replay exposes exact verified run banner and metadata", () => {
  assert.equal(EVIDENCE_RUN_ID, "20261026055631");
  assert.ok(EVIDENCE_BANNER_TEXT.includes("EVIDENCE REPLAY MODE"));
  assert.ok(EVIDENCE_BANNER_TEXT.includes("#20261026055631"));
  assert.equal(REPLAY_HEALTH.status, "replay-snapshot");
  assert.equal(REPLAY_HEALTH.carbonSource, "static-regional-fallback");
  assert.equal(REPLAY_HEALTH.pricingSources?.aws, "bundled-catalog-fallback");
});

test("replay savings exclude failed HPA-conflict proposal and reconcile to $18.25", () => {
  const seriesSum = REPLAY_SAVINGS.series.reduce((acc, p) => acc + p.usd, 0);
  assert.equal(Number(seriesSum.toFixed(2)), 18.25);
  assert.equal(REPLAY_SAVINGS.totalUSD, 18.25);

  const failed = REPLAY_PROPOSALS.find((p) => p.phase === "Failed");
  assert.ok(failed);
  assert.ok(failed.rationale?.includes("autoscaler conflict"));
});

test("replay proposal detail resolves verified artifacts and mutations fail closed", () => {
  const detail = getReplayProposalDetail("e2e-proposal-multinode-tls-20261026055631");
  assert.ok(detail);
  assert.equal(detail.observed.cpu, "250m");
  assert.equal(detail.proposed.cpu, "200m");
  assert.equal(REPLAY_POLICIES.length, 2);

  const err = replayMutationBlockedError();
  assert.match(err.message, /Read-Only Evidence Replay Mode/);
});

test("replay what-if projection flags aggressive cuts with high risk", () => {
  const safe = computeReplayWhatIf({
    namespace: "live-demo",
    workloadName: "storefront",
    currentCPU: "250m",
    currentMemory: "192Mi",
    proposedCPU: "200m",
    proposedMemory: "160Mi",
  });
  assert.equal(safe.allowed, true);
  assert.equal(safe.riskScore, 15);

  const aggressive = computeReplayWhatIf({
    namespace: "live-demo",
    workloadName: "storefront",
    currentCPU: "1000m",
    currentMemory: "1Gi",
    proposedCPU: "200m",
    proposedMemory: "256Mi",
  });
  assert.equal(aggressive.allowed, false);
  assert.ok(aggressive.riskScore >= 70);
});

test("evidence replay includes 2026-11-14 CI, 2-cluster federation, and GitOps PR #4 records", () => {
  assert.equal(EVIDENCE_VERIFIED_RUNS.verifiedCommit, "0ff1e80");
  assert.equal(EVIDENCE_VERIFIED_RUNS.ciE2ERunId, "43612195251");
  assert.equal(EVIDENCE_VERIFIED_RUNS.federationRunId, "43612195323");
  assert.equal(EVIDENCE_VERIFIED_RUNS.federationModelHash, "b3277814cf4dbbfe");
  assert.equal(EVIDENCE_VERIFIED_RUNS.federationOutlierRejectedCount, 1);

  assert.equal(REPLAY_CLUSTERS.length, 2);
  const spoke = REPLAY_CLUSTERS.find((c) => c.clusterName === "spoke-kind-1");
  assert.ok(spoke);
  assert.equal(spoke.status, "Ready");

  const pr4Detail = getReplayProposalDetail("cpu-burner-gitops-pr-4");
  assert.ok(pr4Detail);
  assert.equal(pr4Detail.summary.externalPRID, "Keninjavelas/Reconcilyx#4");
  assert.equal(pr4Detail.summary.phase, "Rejected");
  assert.equal(pr4Detail.events[0]?.reason, "ApprovalRejected");
  assert.equal(pr4Detail.events[0]?.type, "Warning");

  assert.ok(REPLAY_AUDIT.some((a) => a.reason === "FederationSnapshotPromoted" && a.action.includes("b3277814cf4dbbfe")));
  assert.ok(REPLAY_AUDIT.some((a) => a.reason === "CIGatesVerified" && a.action.includes("#43612195251")));
});
