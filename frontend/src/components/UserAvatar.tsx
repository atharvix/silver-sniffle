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
      className={`relative rounded-full overflow-hidden bg-white/10 shrink-0 border border-white/15 flex items-center justify-center font-bold text-white select-none ${className}`}
    >
      {avatar && resolved !== DEFAULT_AVATAR_WEBP && !hasError ? (
        <img
          src={resolved}
          alt={name || ''}
          onError={() => setHasError(true)}
          className="w-full h-full object-cover"
        />
      ) : initial ? (
        <span className="text-white font-semibold">{initial}</span>
      ) : (
        <User className="w-1/2 h-1/2 text-white/50" />
      )}
    </div>
  );
};
