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
        <div className="fixed inset-0 z-50 flex flex-col items-center justify-center p-6 bg-[#060608] text-white text-center select-none">
          <div className="w-20 h-20 rounded-3xl bg-red-500/10 border border-red-500/20 flex items-center justify-center mb-6 shadow-2xl">
            <AlertTriangle className="w-10 h-10 text-red-400" />
          </div>

          <h1 className="text-2xl font-extrabold tracking-tight mb-2">Something went wrong</h1>
          <p className="text-sm text-white/60 max-w-sm mb-6 leading-relaxed">
            {this.state.error?.message || 'An unexpected error occurred while rendering the application.'}
          </p>

          <button
            type="button"
            onClick={this.handleReload}
            className="flex items-center gap-2 px-6 py-3.5 rounded-2xl bg-white hover:bg-neutral-200 text-black font-bold text-sm transition-all active:scale-95 shadow-xl"
          >
            <RotateCw className="w-4 h-4" />
            <span>Reload Application</span>
          </button>
        </div>
      );
    }

    return this.props.children;
  }
}
