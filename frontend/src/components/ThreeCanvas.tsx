import React, { useEffect, useRef } from 'react';
import * as THREE from 'three';
import type { UserProfile } from '../types';

interface ThreeCanvasProps {
  profiles: UserProfile[];
  selectedRadius: number;
  activeProfileId?: string;
  onSelectProfile?: (profile: UserProfile) => void;
}

export const ThreeCanvas: React.FC<ThreeCanvasProps> = ({
  profiles,
  selectedRadius,
  activeProfileId,
}) => {
  const mountRef = useRef<HTMLDivElement>(null);
  const mouseRef = useRef<{ x: number; y: number }>({ x: 0, y: 0 });

  useEffect(() => {
    const container = mountRef.current;
    if (!container) return;

    // 1. Scene, Camera, Renderer setup
    const width = container.clientWidth || window.innerWidth;
    const height = container.clientHeight || window.innerHeight;

    const scene = new THREE.Scene();
    scene.fog = new THREE.FogExp2(0x0a0b0e, 0.015);

    const camera = new THREE.PerspectiveCamera(50, width / height, 0.1, 1000);
    camera.position.set(0, 35, 45);
    camera.lookAt(0, 0, 0);

    const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true });
    renderer.setSize(width, height);
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    renderer.shadowMap.enabled = true;
    container.appendChild(renderer.domElement);

    // 2. Lighting
    const ambientLight = new THREE.AmbientLight(0xffffff, 0.6);
    scene.add(ambientLight);

    const pointLight = new THREE.PointLight(0xe63946, 2, 100);
    pointLight.position.set(0, 20, 0);
    scene.add(pointLight);

    const emeraldLight = new THREE.PointLight(0x10b981, 1.5, 80);
    emeraldLight.position.set(-20, 15, -20);
    scene.add(emeraldLight);

    // 3. Center User Node (Radar Core)
    const coreGroup = new THREE.Group();
    const coreGeo = new THREE.SphereGeometry(1.2, 32, 32);
    const coreMat = new THREE.MeshStandardMaterial({
      color: 0xe63946,
      emissive: 0x8b0000,
      roughness: 0.2,
      metalness: 0.8,
    });
    const coreMesh = new THREE.Mesh(coreGeo, coreMat);
    coreGroup.add(coreMesh);

    // Core pulsing ring
    const ringGeo = new THREE.RingGeometry(1.8, 2.1, 32);
    const ringMat = new THREE.MeshBasicMaterial({
      color: 0xe63946,
      side: THREE.DoubleSide,
      transparent: true,
      opacity: 0.6,
    });
    const coreRing = new THREE.Mesh(ringGeo, ringMat);
    coreRing.rotation.x = Math.PI / 2;
    coreGroup.add(coreRing);
    scene.add(coreGroup);

    // 4. Concentric Radar Grid Rings
    const radarGridGroup = new THREE.Group();
    const radii = [10, 20, 30, 40];
    radii.forEach((r, idx) => {
      const circleGeo = new THREE.RingGeometry(r - 0.08, r + 0.08, 64);
      const circleMat = new THREE.MeshBasicMaterial({
        color: idx === radii.length - 1 ? 0xe63946 : 0x334155,
        side: THREE.DoubleSide,
        transparent: true,
        opacity: idx === radii.length - 1 ? 0.4 : 0.25,
      });
      const ringMesh = new THREE.Mesh(circleGeo, circleMat);
      ringMesh.rotation.x = Math.PI / 2;
      radarGridGroup.add(ringMesh);
    });
    scene.add(radarGridGroup);

    // 5. Background Particle Constellation
    const particleCount = 250;
    const particleGeo = new THREE.BufferGeometry();
    const positions = new Float32Array(particleCount * 3);
    for (let i = 0; i < particleCount * 3; i += 3) {
      positions[i] = (Math.random() - 0.5) * 120;
      positions[i + 1] = (Math.random() - 0.5) * 60;
      positions[i + 2] = (Math.random() - 0.5) * 120;
    }
    particleGeo.setAttribute('position', new THREE.BufferAttribute(positions, 3));
    const particleMat = new THREE.PointsMaterial({
      color: 0x94a3b8,
      size: 0.6,
      transparent: true,
      opacity: 0.5,
    });
    const particles = new THREE.Points(particleGeo, particleMat);
    scene.add(particles);

    // 6. Nearby Profile Nodes in 3D Space
    const nodeMeshes: { mesh: THREE.Mesh; profile: UserProfile; initialPos: THREE.Vector3 }[] = [];
    const nodesGroup = new THREE.Group();

    profiles.forEach((profile, index) => {
      const angle = (index / profiles.length) * Math.PI * 2 + 0.5;
      const mappedDist = Math.min(38, Math.max(8, (profile.distanceMeters / (selectedRadius || 100)) * 36));
      const x = Math.cos(angle) * mappedDist;
      const z = Math.sin(angle) * mappedDist;
      const y = Math.sin(index * 1.5) * 2;

      const nodeGeo = new THREE.SphereGeometry(1.0, 24, 24);
      const isActive = profile.id === activeProfileId;
      const nodeMat = new THREE.MeshStandardMaterial({
        color: isActive ? 0x10b981 : 0x38bdf8,
        emissive: isActive ? 0x064e3b : 0x0369a1,
        roughness: 0.3,
        metalness: 0.6,
      });

      const mesh = new THREE.Mesh(nodeGeo, nodeMat);
      mesh.position.set(x, y, z);
      mesh.userData = { profileId: profile.id, profile };

      const orbitGeo = new THREE.RingGeometry(1.4, 1.6, 24);
      const orbitMat = new THREE.MeshBasicMaterial({
        color: isActive ? 0x10b981 : 0x38bdf8,
        side: THREE.DoubleSide,
        transparent: true,
        opacity: 0.6,
      });
      const orbitMesh = new THREE.Mesh(orbitGeo, orbitMat);
      orbitMesh.rotation.x = Math.PI / 2;
      mesh.add(orbitMesh);

      nodesGroup.add(mesh);
      nodeMeshes.push({ mesh, profile, initialPos: new THREE.Vector3(x, y, z) });
    });
    scene.add(nodesGroup);

    // Mouse Move listener
    const handleMouseMove = (e: MouseEvent) => {
      const rect = container.getBoundingClientRect();
      const x = ((e.clientX - rect.left) / rect.width) * 2 - 1;
      const y = -((e.clientY - rect.top) / rect.height) * 2 + 1;
      mouseRef.current = { x, y };
    };

    window.addEventListener('mousemove', handleMouseMove);

    // Animation Loop
    let animationFrameId: number;
    let clock = new THREE.Clock();

    const animate = () => {
      animationFrameId = requestAnimationFrame(animate);
      const elapsedTime = clock.getElapsedTime();

      const targetCamX = mouseRef.current.x * 6;
      const targetCamZ = 45 + mouseRef.current.y * 6;
      camera.position.x += (targetCamX - camera.position.x) * 0.05;
      camera.position.z += (targetCamZ - camera.position.z) * 0.05;
      camera.lookAt(0, 0, 0);

      particles.rotation.y = elapsedTime * 0.02;
      coreRing.scale.setScalar(1 + Math.sin(elapsedTime * 3) * 0.15);

      nodeMeshes.forEach(({ mesh, initialPos }, idx) => {
        mesh.position.y = initialPos.y + Math.sin(elapsedTime * 2 + idx) * 0.6;
        mesh.rotation.y += 0.01;
      });

      renderer.render(scene, camera);
    };

    animate();

    const handleResize = () => {
      if (!container) return;
      const newW = container.clientWidth || window.innerWidth;
      const newH = container.clientHeight || window.innerHeight;
      camera.aspect = newW / newH;
      camera.updateProjectionMatrix();
      renderer.setSize(newW, newH);
    };

    window.addEventListener('resize', handleResize);

    return () => {
      cancelAnimationFrame(animationFrameId);
      window.removeEventListener('mousemove', handleMouseMove);
      window.removeEventListener('resize', handleResize);
      if (container.contains(renderer.domElement)) {
        container.removeChild(renderer.domElement);
      }
      renderer.dispose();
    };
  }, [profiles, selectedRadius, activeProfileId]);

  return (
    <div
      ref={mountRef}
      className="absolute inset-0 pointer-events-none z-0 overflow-hidden"
    />
  );
};
