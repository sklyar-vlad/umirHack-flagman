"""Reject silently skipped integration suites as well as test failures."""
import json
from pathlib import Path
import sys


OPTIONAL = {
    "TestBackupRestore",
    "TestMonitoringRoleProvisioningAndForbiddenWrites",
    "TestPrometheusScrapesProtectedBackend",
    "TestHTTPSDeploymentCookiesCORSAndStreaming",
    "TestCurlUnbufferedThroughNginxAndTraefik",
}
REQUIRED = {
    "internal/service/order": {
        "TestOrderCreateIntegrationConcurrentOrdersShareProduct",
        "TestOrderCreateIntegrationRollsBackEveryWriteFailure",
        "TestOrderCreateIntegrationRollsBackCommitFailure",
    },
    "internal/service/supply": {"TestCreateSupplyRollbackIntegration"},
    "internal/handler/product": {"TestProductListHTTPIntegration"},
    "internal/service/events": {"TestPublicationCommitOrderRestartAndConcurrentDispatch"},
    "database": {"TestReadyMigrationStateAndMissingSchema"},
}
PREFIX = "github.com/sklyar-vlad/umirHack-flagman/"


def review(lines):
    passed = set()
    problems = []
    skipped = 0
    for line in lines:
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            problems.append("Non-JSON test runner output")
            continue
        package = event.get("Package", "").removeprefix(PREFIX)
        test = event.get("Test", "")
        action = event.get("Action")
        if action == "pass" and test:
            passed.add((package, test))
        elif action == "fail":
            problems.append(f"Failed: {package} {test}")
        elif action == "skip" and test:
            if test not in OPTIONAL:
                problems.append(f"Unexpected skip: {package} {test}")
            skipped += 1
    for package, tests in REQUIRED.items():
        for test in sorted(tests):
            if (package, test) not in passed:
                problems.append(f"Required transaction test did not pass: {package} {test}")
    return passed, skipped, problems


def main():
    with Path(sys.argv[1]).open() as source:
        passed, skipped, problems = review(source)
    if problems:
        print("\n".join(problems), file=sys.stderr)
        return 1
    print(f"Passed {len(passed)} tests/subtests; {skipped} optional proxy/operations tests skipped")
    return 0


if __name__ == "__main__":
    sys.exit(main())
