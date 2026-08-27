import React from 'react';

export const ConstellationBackground: React.FC = () => {
  return (
    <div className="fixed inset-0 pointer-events-none z-0 bg-[#05040a] transition-colors duration-500 overflow-hidden">
      {/* Radial Ambient Cosmic Indigo & Violet Light Spheres */}
      <div className="absolute -top-40 left-1/2 -translate-x-1/2 w-[600px] h-[500px] bg-gradient-to-b from-indigo-900/25 via-purple-900/15 to-transparent rounded-full blur-[120px] opacity-70" />
      <div className="absolute -bottom-40 left-1/4 w-[500px] h-[400px] bg-gradient-to-t from-violet-900/20 via-indigo-900/10 to-transparent rounded-full blur-[100px] opacity-60" />
    </div>
  );
};
