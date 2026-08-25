import { Component, type ReactNode } from 'react'

interface Props {
  children: ReactNode
}

interface State {
  hasError: boolean
  error: Error | null
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props)
    this.state = { hasError: false, error: null }
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    console.error('ErrorBoundary caught:', error, info)
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex h-screen items-center justify-center bg-slate-950 p-8">
          <div className="max-w-lg rounded-lg border-2 border-rose-600 border-t-4 bg-slate-900 p-6 font-mono text-sm">
            <h2 className="mb-4 text-lg font-semibold text-rose-400">[ render_error ]</h2>
            <pre className="mb-4 whitespace-pre-wrap break-words text-slate-300">
              {this.state.error?.message || 'Unknown error'}
            </pre>
            <pre className="mb-4 whitespace-pre-wrap break-words text-xs text-slate-500">
              {this.state.error?.stack}
            </pre>
            <button
              type="button"
              onClick={() => this.setState({ hasError: false, error: null })}
              className="rounded-lg bg-blue-600 px-4 py-2 font-mono text-sm font-medium text-white hover:bg-blue-500"
            >
              retry
            </button>
          </div>
        </div>
      )
    }
    return this.props.children
  }
}
