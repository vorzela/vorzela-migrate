import { signal } from '@vorzelajs/core'
import { Show } from '@vorzelajs/dom'

import { Alert } from '@/components/ui/alert.tsx'
import { AlertDialog } from '@/components/ui/alert-dialog.tsx'
import { Badge } from '@/components/ui/badge.tsx'
import { CodeBlock } from '@/components/ui/code-block.tsx'
import { DiffViewer } from '@/components/ui/diff-viewer.tsx'
import {
  cancelPendingPlan,
  confirmPendingPlan,
  dismissResult,
  lastResult,
  pendingPlan,
  showToast,
} from '@/lib/state.ts'
import { riskLabel } from '@/lib/format.ts'
import type { Plan } from '@/lib/api-types.ts'

function actionText(plan: Plan): string {
  switch (plan.action) {
    case 'project.add':
      return 'Add project'
    case 'project.init':
      return 'Create vm.yaml'
    case 'project.update':
      return 'Update project'
    case 'project.remove':
      return 'Remove project'
    case 'connection.save':
      return 'Save connection'
    case 'connection.delete':
      return 'Delete connection'
    case 'migration.create':
      return 'Create migration'
    case 'migration.save':
      return 'Save file'
    case 'migration.apply_edits':
      return 'Apply drift'
    case 'column.add':
      return 'Add column'
    case 'migrate.run':
      return 'Run migrations'
    case 'rollback.run':
      return 'Roll back'
    case 'fresh.run':
      return 'Rebuild database'
    case 'system.upgrade':
      return 'Upgrade vm'
    default:
      return 'Confirm'
  }
}

function riskVariant(risk: string): 'secondary' | 'warning' | 'destructive' {
  if (risk === 'critical' || risk === 'high') return 'destructive'
  if (risk === 'medium') return 'warning'
  return 'secondary'
}

function PlanDetails({ plan }: { plan: Plan }) {
  return (
    <div class="plan-body">
      <div class="row">
        <Badge variant={riskVariant(plan.risk)}>{riskLabel(plan.risk)}</Badge>
        <span class="muted small">
          Expires {new Date(plan.expiresAt).toLocaleTimeString()}
        </span>
      </div>

      <dl class="plan-target">
        {plan.target.projectName ? (
          <>
            <dt>Project</dt>
            <dd>
              {plan.target.projectName}
              {plan.target.projectPath ? (
                <div class="muted small path">{plan.target.projectPath}</div>
              ) : null}
            </dd>
          </>
        ) : null}
        {plan.target.database ? (
          <>
            <dt>Database</dt>
            <dd>
              {plan.target.connectionName ? (
                <strong>{plan.target.connectionName}</strong>
              ) : null}
              <div class="muted small path">{plan.target.database}</div>
            </dd>
          </>
        ) : null}
        {plan.target.file ? (
          <>
            <dt>Migration</dt>
            <dd class="filename">{plan.target.file}</dd>
          </>
        ) : null}
        {plan.command ? (
          <>
            <dt>Command</dt>
            <dd>
              <code>{plan.command}</code>
            </dd>
          </>
        ) : null}
      </dl>

      {plan.warnings && plan.warnings.length > 0 ? (
        <Alert variant="warning" title="Before you confirm">
          <ul class="plan-warning">
            {plan.warnings.map((warning) => (
              <li key={warning}>{warning}</li>
            ))}
          </ul>
        </Alert>
      ) : null}

      {plan.analysis ? (
        <Alert
          variant={
            plan.analysis.verdict === 'destructive'
              ? 'error'
              : plan.analysis.verdict === 'needs_migration'
                ? 'warning'
                : 'info'
          }
          title={plan.analysis.summary}
        />
      ) : null}

      {plan.preview?.kind === 'diff' ? (
        <DiffViewer
          oldValue={plan.preview.old ?? ''}
          newValue={plan.preview.new ?? ''}
          label={plan.preview.title ?? 'Changes'}
        />
      ) : plan.preview?.content ? (
        <CodeBlock
          code={plan.preview.content}
          title={plan.preview.title ?? 'Preview'}
          className="plan-code"
        />
      ) : null}
    </div>
  )
}

export function PlanDialog() {
  return (
    <Show when={() => pendingPlan() !== null}>
      {() => {
        const plan = pendingPlan() as Plan
        const [phrase, setPhrase] = signal('')
        const [error, setError] = signal('')
        const [open, setOpen] = signal(true)

        function close(): void {
          setOpen(false)
          cancelPendingPlan()
        }

        async function run(): Promise<void> {
          const typed = phrase().trim()
          if (plan.requiresTypedConfirmation && typed !== plan.typingPhrase) {
            setError('Type the exact confirmation phrase shown above.')
            return
          }
          setError('')
          const result = await confirmPendingPlan(typed)
          if (!result) {
            // confirmPendingPlan already surfaced the error/result.
            setOpen(false)
          }
        }

        return (
          <AlertDialog
            open={() => open()}
            onOpenChange={(next) => {
              if (!next) close()
            }}
            className="plan-dialog"
            title={plan.title}
            description={plan.summary}
            cancelText="Cancel"
            actionText={actionText(plan)}
            destructive={plan.risk === 'high' || plan.risk === 'critical'}
            onCancel={close}
            onAction={() => void run()}
          >
            <PlanDetails plan={plan} />

            {plan.requiresTypedConfirmation ? (
              <div class="phrase-row">
                <label for="confirm-phrase">
                  Type <strong>{plan.typingPhrase}</strong> to confirm this
                  {plan.risk === 'critical' ? ' critical' : ''} action
                </label>
                <input
                  id="confirm-phrase"
                  autocomplete="off"
                  spellcheck={false}
                  value={() => phrase()}
                  onInput={(event) =>
                    setPhrase((event.target as HTMLInputElement).value)
                  }
                />
                <Show when={() => error() !== ''}>
                  <p class="phrase-error" role="alert">
                    {() => error()}
                  </p>
                </Show>
              </div>
            ) : null}
          </AlertDialog>
        )
      }}
    </Show>
  )
}

function combinedOutput(): string {
  const value = lastResult()
  if (!value) return ''
  const { result } = value
  return [
    result.command ? `$ ${result.command}` : '',
    result.stdout ?? '',
    result.stderr ?? '',
  ]
    .filter(Boolean)
    .join('\n')
}

export function ResultDialog() {
  return (
    <Show when={() => lastResult() !== null}>
      {() => {
        const value = lastResult() as NonNullable<ReturnType<typeof lastResult>>
        const { plan, result } = value
        const [open, setOpen] = signal(true)
        const output = [result.stdout, result.stderr].filter(Boolean).join('\n')

        return (
          <AlertDialog
            open={() => open()}
            onOpenChange={(next) => {
              if (!next) {
                setOpen(false)
                dismissResult()
              }
            }}
            className="plan-dialog"
            title={result.ok ? 'Action complete' : 'Action failed'}
            description={
              result.message ??
              (result.ok
                ? 'The confirmed action finished.'
                : 'The action did not finish cleanly. Review the output.')
            }
            cancelText="Close"
            actionText="Copy output"
            onCancel={() => {
              setOpen(false)
              dismissResult()
            }}
            onAction={() => {
              void navigator.clipboard.writeText(combinedOutput())
              showToast('success', 'Output copied')
            }}
          >
            <div class="plan-body">
              <div class="row">
                <Badge variant={result.ok ? 'success' : 'destructive'}>
                  {result.ok ? 'Success' : 'Failed'}
                </Badge>
                <span class="muted small">
                  exit {result.exitCode} · {result.durationMs} ms
                </span>
                {plan.target.database ? (
                  <span class="chip">
                    <strong>DB</strong> {plan.target.database}
                  </span>
                ) : null}
              </div>
              {result.warnings && result.warnings.length > 0 ? (
                <ul class="plan-warning">
                  {result.warnings.map((warning) => (
                    <li key={warning}>{warning}</li>
                  ))}
                </ul>
              ) : null}
              {output ? (
                <div class="result-output">
                  <CodeBlock
                    code={output}
                    title={result.command ?? 'Output'}
                    copyable={false}
                  />
                </div>
              ) : (
                <p class="muted">No terminal output.</p>
              )}
            </div>
          </AlertDialog>
        )
      }}
    </Show>
  )
}
