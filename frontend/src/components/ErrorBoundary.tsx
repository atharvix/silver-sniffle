import { Component } from 'react';
import type { ErrorInfo, ReactNode } from 'react';
import { RotateCw, AlertTriangle } from 'lucide-react';

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  public state: State = {
    hasError: false,
    error: null,
  };

  public static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  public componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error('Uncaught error caught by ErrorBoundary:', error, errorInfo);
  }

  private handleReload = () => {
    this.setState({ hasError: false, error: null });
    window.location.reload();
  };

  public render() {
    if (this.state.hasError) {
      return (
        <div className="fixed inset-0 z-50 flex flex-col items-center justify-center p-6 text-center select-none" style={{ background: 'var(--bg)', color: 'var(--fg)' }}>
          <div className="w-16 h-16 rounded-full flex items-center justify-center mb-6" style={{ background: 'rgba(255,93,93,0.1)', border: '1px solid rgba(255,93,93,0.3)' }}>
            <AlertTriangle className="w-7 h-7" style={{ color: 'var(--danger)' }} />
          </div>

          <h1 className="text-[28px] font-normal tracking-tight mb-2" style={{ letterSpacing: '-.04em' }}>Something went wrong</h1>
          <p className="text-sm max-w-sm mb-6 leading-relaxed" style={{ color: 'var(--muted)' }}>
            {this.state.error?.message || 'An unexpected error occurred while rendering the application.'}
          </p>

          <button
            type="button"
            onClick={this.handleReload}
            className="btn"
            style={{ width: 'auto', height: 48, padding: '0 24px' }}
          >
            <RotateCw className="w-4 h-4" />
            <span>Reload application</span>
          </button>
        </div>
      );
    }

    return this.props.children;
  }
}
