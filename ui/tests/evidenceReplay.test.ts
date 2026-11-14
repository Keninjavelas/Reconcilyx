import test from "node:test";
import assert from "node:assert/strict";
import {
  EVIDENCE_BANNER_TEXT,
  EVIDENCE_RUN_ID,
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
