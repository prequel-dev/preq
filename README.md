# preq

`preq` (prounounced "preek") is a free and open community-driven reliability problem detector

[Documentation](https://docs.prequel.dev) | [Slack](https://inviter.co/prequel) | [Playground](https://play.prequel.dev/) | [Mailing List](https://www.detect.sh)

[![Unit Tests](https://github.com/prequel-dev/cre/actions/workflows/build.yml/badge.svg)](https://github.com/prequel-dev/cre/actions/workflows/build.yml)
[![Unit Tests](https://github.com/prequel-dev/preq/actions/workflows/build.yml/badge.svg)](https://github.com/prequel-dev/preq/actions/workflows/build.yml)
[![Unit Tests](https://github.com/prequel-dev/prequel-compiler/actions/workflows/build.yml/badge.svg)](https://github.com/prequel-dev/prequel-compiler/actions/workflows/build.yml)
[![Coverage](https://img.shields.io/badge/Coverage-0%25-red.svg)](https://github.com/prequel-dev/preq/actions/workflows/coverage.yml)

---

Use `preq` to:

- detect the latest bugs, misconfigurations, anti-patterns, and known issues from a community of practitioners
- provide engineers, on-call support, and SRE agents with impact and community recommended mitigations
- hunt for new problems in distributed systems

## Install

See https://docs.prequel.dev/install.

## Overview

`preq` uses Common Reliability Enumerations (CREs) created by the problem detection community and Prequel's Reliability Research Team to detect reliability problems. Reliability Intelligence from CREs helps teams see the most problems and see them first so they can prioritize, pinpoint, and act to mitigate outages.

`preq` is powered by a rules engine that performs distributed matching and correlation of sequences of events across logs, metrics, traces, and other data sources to detect reliability problems. CREs provides accurate and timely context for a human or SRE agent to take action on problems.

Below is simple rule that looks for a sequence of events in a single log source over a window of time along with a negative condition (an event that should not occur during the window).

```yaml title="cre-2024-0007.yaml" showLineNumbers
cre:
  id: CRE-2024-0007
  severity: 0
  title: RabbitMQ Mnesia overloaded recovering persistent queues
  category: message-queue-problems
  author: Prequel
  description: |
    - The RabbitMQ cluster is processing a large number of persistent mirrored queues at boot. 
  cause: |
    - The Erlang process, Mnesia, is overloaded while recovering persistent queues on boot. 
  impact: |
    - RabbitMQ is unable to process any new messages and can cause outages in consumers and producers.
  tags: 
    - cre-2024-0007
    - known-problem
    - rabbitmq
  mitigation: |
    - Adjusting mirroring policies to limit the number of mirrored queues
    - Remove high-availability policies from queues
    - Add additional CPU resources and restart the RabbitMQ cluster
    - Use [lazy queues](https://www.rabbitmq.com/docs/lazy-queues) to avoid incurring the costs of writing data to disk 
  references:
    - https://groups.google.com/g/rabbitmq-users/c/ekV9tTBRZms/m/1EXw-ruuBQAJ
  applications:
    - name: "rabbitmq"
      version: "3.9.x"
metadata:
  kind: prequel
  id: 5UD1RZxGC5LJQnVpAkV11A
  generation: 1
rule:
  sequence:
    window: 30s
    event:
      src: log
      container_name: rabbitmq
    order:
      - regex: Discarding message(.+)in an old incarnation(.+)of this node
      - Mnesia is overloaded
    negate:
      - SIGTERM received - shutting down
```
