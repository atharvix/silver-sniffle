import React, { useState, useEffect } from 'react';
import { resolvePhotoUrl, DEFAULT_AVATAR_WEBP } from '../utils/api';
import { User } from 'lucide-react';

interface UserAvatarProps {
  avatar?: string;
  name?: string;
  className?: string;
  onClick?: () => void;
}

export const UserAvatar: React.FC<UserAvatarProps> = ({
  avatar,
  name,
  className = 'w-12 h-12',
  onClick,
}) => {
  const [hasError, setHasError] = useState(false);
  const resolved = resolvePhotoUrl(avatar);

  useEffect(() => {
    setHasError(false);
  }, [avatar]);

  const initial = name ? name.trim().charAt(0).toUpperCase() : '';

  return (
    <div
      onClick={onClick}
      className={`relative rounded-full overflow-hidden shrink-0 flex items-center justify-center font-semibold select-none ${className}`}
      style={{ background: 'var(--surface-2)', border: '1.5px solid var(--hairline)', color: 'var(--fg)' }}
    >
      {avatar && resolved !== DEFAULT_AVATAR_WEBP && !hasError ? (
        <img
          src={resolved}
          alt={name || ''}
          onError={() => setHasError(true)}
          className="w-full h-full object-cover"
        />
      ) : initial ? (
        <span>{initial}</span>
      ) : (
        <User className="w-1/2 h-1/2" style={{ color: 'var(--faint)' }} />
      )}
    </div>
  );
};
