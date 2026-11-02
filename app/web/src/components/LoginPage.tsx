import { useState } from 'react';
import { Mail, KeyRound, Loader2, CheckCircle } from 'lucide-react';
import { requestCode, loginWithCode } from '@/lib/api';

interface LoginPageProps {
  onLogin: () => void;
}

type Step = 'initial' | 'code_sent' | 'logging_in' | 'success';

export function LoginPage({ onLogin }: LoginPageProps) {
  const [step, setStep] = useState<Step>('initial');
  const [code, setCode] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const handleRequestCode = async () => {
    setLoading(true);
    setError(null);
    try {
      await requestCode();
      setStep('code_sent');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to send code');
    } finally {
      setLoading(false);
    }
  };

  const handleLogin = async () => {
    if (!code.trim()) return;
    setLoading(true);
    setError(null);
    setStep('logging_in');
    try {
      await loginWithCode(code.trim());
      setStep('success');
      setTimeout(onLogin, 1000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed');
      setStep('code_sent');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen bg-background flex items-center justify-center p-4">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <h1 className="text-2xl font-semibold tracking-tight text-foreground mb-2">Roborock Bridge</h1>
          <p className="text-sm text-muted-foreground">Reconnectez votre compte pour retrouver vos robots.</p>
        </div>

        <div className="bridge-card p-7">
          {step === 'success' ? (
            <div className="text-center py-4">
              <CheckCircle className="h-12 w-12 text-green-500 mx-auto mb-3" />
              <p className="text-foreground font-medium">Vos robots sont connectés.</p>
              <p className="text-sm text-muted-foreground mt-1">Ouverture de votre passerelle…</p>
            </div>
          ) : step === 'initial' ? (
            <>
              <p className="text-sm text-muted-foreground mb-4">
                Recevez un code de connexion à l’adresse e-mail de votre compte Roborock. Vos réglages Loxone sont conservés.
              </p>
              <button
                onClick={handleRequestCode}
                disabled={loading}
                className="w-full p-3 rounded-lg bg-primary text-primary-foreground font-medium hover:opacity-90 transition-opacity disabled:opacity-50 flex items-center justify-center gap-2 touch-target"
              >
                {loading ? (
                  <Loader2 className="h-5 w-5 animate-spin" />
                ) : (
                  <Mail className="h-5 w-5" />
                )}
                {loading ? 'Envoi en cours…' : 'Recevoir mon code'}
              </button>
            </>
          ) : (
            <>
              <p className="text-sm text-muted-foreground mb-4">
                Saisissez le code reçu par e-mail.
              </p>
              <div className="flex gap-2 mb-4">
                <div className="relative flex-1">
                  <KeyRound className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                  <input
                    type="text"
                    inputMode="numeric"
                    value={code}
                    onChange={(e) => setCode(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && handleLogin()}
                    placeholder="Code de connexion"
                    aria-label="Code de connexion Roborock"
                    autoComplete="one-time-code"
                    autoFocus
                    className="w-full pl-10 pr-3 py-3 rounded-lg border border-input bg-background text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                  />
                </div>
              </div>
              <button
                onClick={handleLogin}
                disabled={loading || !code.trim()}
                className="w-full p-3 rounded-lg bg-primary text-primary-foreground font-medium hover:opacity-90 transition-opacity disabled:opacity-50 flex items-center justify-center gap-2 touch-target"
              >
                {loading ? (
                  <Loader2 className="h-5 w-5 animate-spin" />
                ) : (
                  'Connecter mes robots'
                )}
              </button>
              <button
                onClick={handleRequestCode}
                disabled={loading}
                className="w-full mt-2 p-2 text-sm text-muted-foreground hover:text-foreground transition-colors"
              >
                Renvoyer le code
              </button>
            </>
          )}

          {error && (
            <div role="alert" className="mt-4 p-3 bg-red-500/10 border border-red-500/20 rounded-lg text-red-500 text-sm">
              {error}
            </div>
          )}
        </div>

        <div className="mt-6 text-center text-xs text-muted-foreground">
          Votre passerelle Roborock → Loxone
        </div>
      </div>
    </div>
  );
}
