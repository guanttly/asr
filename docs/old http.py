#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
HTTP文件上传服务器 - 处理文件上传和静态文件服务
"""

import asyncio
import json
import logging
import mimetypes
import wave
import numpy as np
import uuid
import aiohttp
import re
from datetime import datetime
from pathlib import Path
from typing import Dict, Any, Optional, List
from io import BytesIO
import sys
from aiohttp import web, web_request
from aiohttp.web_response import Response

# 添加项目根目录到Python路径
project_root = Path(__file__).parent.parent.parent
sys.path.insert(0, str(project_root))

from config import CONFIG
from src.services.service_manager import service_manager
from src.services.task_service import task_service, TaskStatus
from src.utils.audio_utils import AudioProcessor
from src.utils.temp_file_manager import temp_file_manager

logger = logging.getLogger(__name__)


class HTTPServer:
    """HTTP文件服务器"""
    
    def __init__(self, host: str = "localhost", port: int = 8081):
        self.host = host
        self.port = port
        # 创建应用实例，不设置超时（超时将在具体的HTTP请求中处理）
        self.app = web.Application()
        self.asr_service = None  # 将通过service_manager异步获取
        
        # 设置路由
        self._setup_routes()
    
    async def initialize(self):
        """初始化服务"""
        if self.asr_service is None:
            # 使用服务管理器获取单例ASR服务
            self.asr_service = await service_manager.get_asr_service()
        
        # 初始化LLM服务
        try:
            self.llm_service = await service_manager.get_llm_service()
            logger.info("LLM服务初始化成功")
        except Exception as e:
            logger.warning(f"LLM服务初始化失败: {e}")
            self.llm_service = None
        
        # 初始化任务管理服务
        await task_service.initialize()
        
        # 初始化临时文件管理器
        await temp_file_manager.initialize()
        

    def _setup_routes(self):
        """设置路由"""
        # API路由
        self.app.router.add_post('/api/upload', self.handle_file_upload)
        self.app.router.add_post('/api/recognize', self.handle_recognize_audio)
        self.app.router.add_post('/api/recognize/vad', self.handle_recognize_audio_vad)
        self.app.router.add_get('/api/health', self.handle_health_check)
        
        # LLM相关路由
        self.app.router.add_post('/api/meeting/summary', self.handle_meeting_summary)
        self.app.router.add_post('/api/text/correct', self.handle_text_correction)
        self.app.router.add_get('/api/templates', self.handle_get_templates)
        self.app.router.add_post('/api/audio/to_summary', self.handle_audio_to_summary)
        
        # 任务管理路由
        self.app.router.add_get('/api/task/{task_id}', self.handle_get_task_status)
        
        # 静态文件路由
        static_dir = project_root / "examples"
        self.app.router.add_static('/', static_dir, show_index=True)
    
    def _is_valid_url(self, url: str) -> bool:
        """验证URL格式是否正确"""
        if not url or not isinstance(url, str):
            return False
        
        # 基本的HTTP/HTTPS URL格式验证
        url_pattern = re.compile(
            r'^https?://'  # http:// 或 https://
            r'(?:(?:[A-Z0-9](?:[A-Z0-9-]{0,61}[A-Z0-9])?\.)+[A-Z]{2,6}\.?|'  # 域名
            r'localhost|'  # localhost
            r'\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})'  # IP地址
            r'(?::\d+)?'  # 端口号
            r'(?:/?|[/?]\S+)$', re.IGNORECASE)  # 路径
        
        return bool(url_pattern.match(url))

    
    async def handle_get_task_status(self, request: web_request.Request) -> Response:
        """获取任务状态"""
        try:
            task_id = request.match_info['task_id']
            
            task_status = task_service.get_task_status(task_id)
            if task_status:
                return web.json_response({
                    "success": True,
                    "data": task_status
                })
            else:
                return web.json_response({
                    "success": False,
                    "message": "任务不存在或已完成"
                }, status=404)
                
        except Exception as e:
            logger.error(f"获取任务状态失败: {e}")
            return web.json_response({
                "success": False,
                "message": f"获取任务状态失败: {str(e)}"
            }, status=500)
    
    async def handle_file_upload(self, request: web_request.Request) -> Response:
        """处理文件上传"""
        try:
            logger.info("收到文件上传请求")
            
            # 检查Content-Type
            if not request.content_type.startswith('multipart/form-data'):
                return web.json_response({
                    "success": False,
                    "message": "请使用multipart/form-data上传文件"
                }, status=400)
            
            # 读取multipart数据
            reader = await request.multipart()
            
            file_data = None
            filename = None
            language = "auto"
            use_itn = True
            use_vad_segmentation = False
            hotwords = None
            
            async for field in reader:
                if field.name == 'file':
                    filename = field.filename or "uploaded_file"
                    file_data = await field.read()
                    logger.info(f"接收文件: {filename}, 大小: {len(file_data)} bytes")
                    
                    # 保存到临时文件
                    temp_file_path = await temp_file_manager.save_temp_file(
                        file_data, filename
                    )
                elif field.name == 'language':
                    language = (await field.read()).decode('utf-8')
                elif field.name == 'use_itn':
                    use_itn_str = (await field.read()).decode('utf-8')
                    use_itn = use_itn_str.lower() in ['true', '1', 'yes']
                elif field.name == 'use_vad_segmentation':
                    vad_str = (await field.read()).decode('utf-8')
                    use_vad_segmentation = vad_str.lower() in ['true', '1', 'yes']
                elif field.name == 'hotwords':
                    hotwords_str = (await field.read()).decode('utf-8')
                    if hotwords_str.strip():
                        # 支持多种分隔符：逗号、分号、换行符
                        import re
                        hotwords = [word.strip() for word in re.split(r'[,;\n]', hotwords_str) if word.strip()]
            
            if 'temp_file_path' not in locals():
                return web.json_response({
                    "success": False,
                    "message": "未找到上传的文件"
                }, status=400)
            
            try:
                # 处理文件并识别
                result = await self.process_uploaded_file_from_temp(
                    temp_file_path, filename, language, use_itn, use_vad_segmentation, hotwords
                )
                
                return web.json_response(result)
            finally:
                # 清理临时文件
                asyncio.create_task(temp_file_manager.cleanup_temp_file(temp_file_path))
            
        except Exception as e:
            logger.error(f"处理文件上传失败: {e}")
            return web.json_response({
                "success": False,
                "message": f"处理文件失败: {str(e)}"
            }, status=500)

    
    async def handle_health_check(self, request: web_request.Request) -> Response:
        """健康检查"""
        return web.json_response({
            "status": "healthy",
            "timestamp": datetime.now().isoformat(),
            "service": "JushaAsr HTTP Server"
        })
    
    async def handle_recognize_audio(self, request: web_request.Request) -> Response:
        """处理基础音频识别"""
        try:
            logger.info("收到音频识别请求")
            
            # 检查Content-Type
            if not request.content_type.startswith('multipart/form-data'):
                return web.json_response({
                    "success": False,
                    "message": "请使用multipart/form-data上传文件"
                }, status=400)
            
            # 读取multipart数据
            reader = await request.multipart()
            
            file_data = None
            filename = None
            language = "auto"
            use_itn = True
            hotwords = None
            
            async for field in reader:
                if field.name == 'file':
                    filename = field.filename or "uploaded_file"
                    file_data = await field.read()
                    logger.info(f"接收文件: {filename}, 大小: {len(file_data)} bytes")
                    
                    # 保存到临时文件
                    temp_file_path = await temp_file_manager.save_temp_file(
                        file_data, filename
                    )
                elif field.name == 'language':
                    language = (await field.read()).decode('utf-8')
                elif field.name == 'use_itn':
                    use_itn_str = (await field.read()).decode('utf-8')
                    use_itn = use_itn_str.lower() in ['true', '1', 'yes']
                elif field.name == 'hotwords':
                    hotwords_str = (await field.read()).decode('utf-8')
                    if hotwords_str.strip():
                        # 支持多种分隔符：逗号、分号、换行符
                        import re
                        hotwords = [word.strip() for word in re.split(r'[,;\n]', hotwords_str) if word.strip()]
            
            if 'temp_file_path' not in locals():
                return web.json_response({
                    "success": False,
                    "message": "未找到上传的文件"
                }, status=400)
            
            try:
                # 处理文件并识别（使用基础识别）
                result = await self.process_uploaded_file_from_temp(
                    temp_file_path, filename, language, use_itn, False, hotwords  # use_vad_segmentation=False
                )
                
                return web.json_response(result)
            finally:
                # 清理临时文件
                asyncio.create_task(temp_file_manager.cleanup_temp_file(temp_file_path))
            
        except Exception as e:
            logger.error(f"处理音频识别失败: {e}")
            return web.json_response({
                "success": False,
                "message": f"处理音频识别失败: {str(e)}"
            }, status=500)
    
    async def handle_recognize_audio_vad(self, request: web_request.Request) -> Response:
        """处理VAD分割音频识别"""
        try:
            logger.info("收到VAD分割音频识别请求")
            
            # 检查Content-Type
            if not request.content_type.startswith('multipart/form-data'):
                return web.json_response({
                    "success": False,
                    "message": "请使用multipart/form-data上传文件"
                }, status=400)
            
            # 读取multipart数据
            reader = await request.multipart()
            
            file_data = None
            filename = None
            language = "auto"
            use_itn = True
            hotwords = None
            min_segment_duration = 1.0  # 改为1.0秒，和本地测试一致
            max_segment_duration = 30.0
            
            async for field in reader:
                if field.name == 'file':
                    filename = field.filename or "uploaded_file"
                    file_data = await field.read()
                    logger.info(f"接收文件: {filename}, 大小: {len(file_data)} bytes")
                    
                    # 保存到临时文件
                    temp_file_path = await temp_file_manager.save_temp_file(
                        file_data, filename
                    )
                elif field.name == 'language':
                    language = (await field.read()).decode('utf-8')
                elif field.name == 'use_itn':
                    use_itn_str = (await field.read()).decode('utf-8')
                    use_itn = use_itn_str.lower() in ['true', '1', 'yes']
                elif field.name == 'hotwords':
                    hotwords_str = (await field.read()).decode('utf-8')
                    if hotwords_str.strip():
                        # 支持多种分隔符：逗号、分号、换行符
                        import re
                        hotwords = [word.strip() for word in re.split(r'[,;\n]', hotwords_str) if word.strip()]
                elif field.name == 'min_segment_duration':
                    try:
                        min_segment_duration = float((await field.read()).decode('utf-8'))
                    except (ValueError, TypeError):
                        pass
                elif field.name == 'max_segment_duration':
                    try:
                        max_segment_duration = float((await field.read()).decode('utf-8'))
                    except (ValueError, TypeError):
                        pass
            
            if 'temp_file_path' not in locals():
                return web.json_response({
                    "success": False,
                    "message": "未找到上传的文件"
                }, status=400)
            
            try:
                # 处理文件并识别（使用VAD分割识别）
                result = await self.process_uploaded_file_from_temp(
                    temp_file_path, filename, language, use_itn, True, hotwords, 
                    min_segment_duration, max_segment_duration  # 传递VAD参数
                )
                
                return web.json_response(result)
            finally:
                # 清理临时文件
                asyncio.create_task(temp_file_manager.cleanup_temp_file(temp_file_path))
            
        except Exception as e:
            logger.error(f"处理VAD分割音频识别失败: {e}")
            return web.json_response({
                "success": False,
                "message": f"处理VAD分割音频识别失败: {str(e)}"
            }, status=500)
    
    async def start_server(self):
        """启动服务器"""
        try:
            # 初始化服务
            await self.initialize()
            
            logger.info(f"启动HTTP服务器: http://{self.host}:{self.port}")
            runner = web.AppRunner(self.app)
            await runner.setup()
            
            site = web.TCPSite(runner, self.host, self.port)
            await site.start()
            
            logger.info("HTTP服务器启动成功")
            
            # 保持服务器运行
            try:
                await asyncio.Future()  # 永远等待
            except asyncio.CancelledError:
                logger.info("HTTP服务器收到停止信号")
            finally:
                # 关闭服务
                await self.shutdown()
                await runner.cleanup()
                
        except Exception as e:
            logger.error(f"HTTP服务器启动失败: {e}")
            raise
    
    async def shutdown(self):
        """关闭服务器和相关服务"""
        logger.info("关闭HTTP服务器和相关服务")
        
        try:
            # 关闭任务管理服务
            await task_service.shutdown()
            
            # 关闭临时文件管理器
            await temp_file_manager.shutdown()
            
            logger.info("HTTP服务器和相关服务已关闭")
        except Exception as e:
            logger.error(f"关闭服务时出错: {e}")

    # === 文件处理方法 ===
    
    async def process_uploaded_file_from_temp(self, temp_file_path: Path, filename: str, language: str = "auto", use_itn: bool = True, use_vad_segmentation: bool = False, hotwords: Optional[List[str]] = None, min_segment_duration: float = 1.0, max_segment_duration: float = 30.0) -> Dict[str, Any]:
        """从临时文件处理上传的文件并进行语音识别"""
        try:
            logger.info(f"从临时文件处理上传文件: {filename}, 路径: {temp_file_path}, VAD分割: {use_vad_segmentation}, 热词: {hotwords}")
            
            # 检查文件扩展名
            file_ext = Path(filename).suffix.lower()
            logger.debug(f"文件扩展名: {file_ext}")
            
            # 读取临时文件
            file_data = await temp_file_manager.read_temp_file(temp_file_path)
            
            # 对于支持的音频格式，直接传递给ASR服务处理
            if file_ext in ['.wav', '.mp3', '.flac', '.m4a', '.ogg', '.webm']:
                logger.debug(f"直接使用ASR服务处理{file_ext}格式文件")
                
                # 根据参数选择识别方式
                if use_vad_segmentation:
                    # 使用VAD分割识别
                    result = await self.asr_service.recognize_audio_with_vad_segmentation(
                        file_data, language, use_itn, 
                        min_segment_duration=min_segment_duration,
                        max_segment_duration=max_segment_duration,
                        hotwords=hotwords
                    )
                else:
                    # 使用基础识别
                    result = await self.asr_service.recognize_audio(
                        file_data, language, use_itn, hotwords=hotwords
                    )
                
                return {
                    "success": True,
                    "filename": filename,
                    "language": language,
                    "use_vad_segmentation": use_vad_segmentation,
                    "result": result,
                    "timestamp": datetime.now().isoformat()
                }
            
            # 对于其他格式，尝试转换处理
            else:
                logger.debug(f"尝试转换{file_ext}格式文件")
                # 根据文件扩展名和数据处理音频
                audio_data = await self._process_audio_file(file_data, filename)
                
                if audio_data is None:
                    return {
                        "success": False,
                        "message": "无法从文件中提取音频数据",
                        "timestamp": datetime.now().isoformat()
                    }
                
                # 将 numpy 数组转换为 bytes（用于 ASR 服务）
                wav_bytes = self._numpy_to_wav_bytes(audio_data, CONFIG.audio.sample_rate)
                
                # 根据参数选择识别方式
                if use_vad_segmentation:
                    # 使用VAD分割识别
                    result = await self.asr_service.recognize_audio_with_vad_segmentation(
                        wav_bytes, language, use_itn, 
                        min_segment_duration=min_segment_duration,
                        max_segment_duration=max_segment_duration,
                        hotwords=hotwords
                    )
                else:
                    # 使用基础识别
                    result = await self.asr_service.recognize_audio(
                        wav_bytes, language, use_itn, hotwords=hotwords
                    )
                
                return {
                    "success": True,
                    "filename": filename,
                    "language": language,
                    "use_vad_segmentation": use_vad_segmentation,
                    "result": result,
                    "timestamp": datetime.now().isoformat()
                }
            
        except Exception as e:
            logger.error(f"从临时文件处理文件失败: {e}")
            return {
                "success": False,
                "message": f"处理文件失败: {str(e)}",
                "timestamp": datetime.now().isoformat()
            }
    
    async def _process_audio_file(self, file_data: bytes, filename: str) -> Optional[np.ndarray]:
        """处理音频文件数据"""
        try:
            file_ext = Path(filename).suffix.lower()
            
            # 先尝试使用soundfile处理
            if file_ext in ['.wav', '.mp3', '.flac', '.m4a', '.ogg', '.webm']:
                try:
                    import soundfile as sf
                    file_io = BytesIO(file_data)
                    audio_data, sample_rate = sf.read(file_io)
                    
                    logger.debug(f"使用soundfile读取: {filename}, 采样率: {sample_rate}, 形状: {audio_data.shape}")
                    
                    # 如果是立体声，转换为单声道
                    if len(audio_data.shape) > 1:
                        audio_data = np.mean(audio_data, axis=1)
                    
                    # 标准化音频
                    # 使用AudioProcessor标准化音频（转换为bytes格式）
                    wav_data = self._numpy_to_wav_bytes(audio_data, sample_rate)
                    normalized_wav = AudioProcessor.normalize_audio(wav_data, CONFIG.audio.sample_rate)
                    
                    # 将标准化后的WAV转换回numpy数组
                    normalized_audio = self._wav_bytes_to_numpy(normalized_wav)
                    
                    return normalized_audio
                    
                except ImportError:
                    logger.warning("soundfile库未安装，尝试其他方法")
                except Exception as sf_error:
                    logger.warning(f"soundfile处理失败: {sf_error}")
            
            # 尝试作为WAV文件处理
            if file_ext == '.wav' or self._is_wav_format(file_data):
                return await self._process_wav_file(file_data)
            
            # 尝试作为原始PCM数据处理
            if file_ext in ['.pcm', '.raw']:
                return await self._process_pcm_file(file_data)
            
            raise ValueError(f"不支持的音频文件格式: {file_ext}")
            
        except Exception as e:
            logger.error(f"处理音频文件失败: {e}")
            return None
    
    def _is_wav_format(self, data: bytes) -> bool:
        """检查是否为WAV格式"""
        return data.startswith(b'RIFF') and b'WAVE' in data[:12]
    
    async def _process_wav_file(self, file_data: bytes) -> Optional[np.ndarray]:
        """处理WAV文件"""
        try:
            file_io = BytesIO(file_data)
            with wave.open(file_io, 'rb') as wav_file:
                frames = wav_file.readframes(-1)
                sample_rate = wav_file.getframerate()
                channels = wav_file.getnchannels()
                sample_width = wav_file.getsampwidth()
                
                logger.debug(f"WAV文件信息: 采样率={sample_rate}, 声道={channels}, 位深={sample_width}")
                
                # 根据采样位深选择数据类型
                if sample_width == 1:
                    dtype = np.uint8
                    max_val = 128
                    offset = 128
                elif sample_width == 2:
                    dtype = np.int16
                    max_val = 32768
                    offset = 0
                elif sample_width == 4:
                    dtype = np.int32
                    max_val = 2147483648
                    offset = 0
                else:
                    raise ValueError(f"不支持的采样位深: {sample_width}")
                
                # 转换为numpy数组
                audio_data = np.frombuffer(frames, dtype=dtype).astype(np.float32)
                
                # 归一化到[-1, 1]
                if sample_width == 1:
                    audio_data = (audio_data - offset) / max_val
                else:
                    audio_data = audio_data / max_val
                
                # 处理多声道
                if channels > 1:
                    audio_data = audio_data.reshape(-1, channels)
                    audio_data = np.mean(audio_data, axis=1)
                
                # 重采样到目标采样率
                if sample_rate != CONFIG.audio.sample_rate:
                    # 将numpy数组转换为WAV bytes再进行采样率转换
                    wav_data = self._numpy_to_wav_bytes(audio_data, sample_rate)
                    resampled_wav = AudioProcessor.convert_sample_rate(wav_data, CONFIG.audio.sample_rate)
                    audio_data = self._wav_bytes_to_numpy(resampled_wav)
                
                return audio_data
                
        except Exception as e:
            logger.error(f"处理WAV文件失败: {e}")
            return None
    
    async def _process_pcm_file(self, file_data: bytes) -> Optional[np.ndarray]:
        """处理PCM文件"""
        try:
            # 假设是16位PCM数据
            audio_data = np.frombuffer(file_data, dtype=np.int16).astype(np.float32)
            audio_data = audio_data / 32768.0  # 归一化到[-1, 1]
            
            logger.debug(f"PCM数据长度: {len(audio_data)}")
            
            return audio_data
            
        except Exception as e:
            logger.error(f"处理PCM文件失败: {e}")
            return None
    
    def _numpy_to_wav_bytes(self, audio_data: np.ndarray, sample_rate: int) -> bytes:
        """将numpy音频数组转换为WAV格式的bytes"""
        try:
            # 确保数据在[-1, 1]范围内
            audio_data = np.clip(audio_data, -1.0, 1.0)
            
            # 转换为16位整数
            audio_int16 = (audio_data * 32767).astype(np.int16)
            
            # 创建WAV文件
            wav_io = BytesIO()
            with wave.open(wav_io, 'wb') as wav_file:
                wav_file.setnchannels(1)  # 单声道
                wav_file.setsampwidth(2)  # 16位
                wav_file.setframerate(sample_rate)
                wav_file.writeframes(audio_int16.tobytes())
            
            return wav_io.getvalue()
            
        except Exception as e:
            logger.error(f"转换numpy到WAV失败: {e}")
            return b''
    
    def _wav_bytes_to_numpy(self, wav_data: bytes) -> Optional[np.ndarray]:
        """将WAV格式的bytes转换为numpy数组"""
        try:
            wav_io = BytesIO(wav_data)
            with wave.open(wav_io, 'rb') as wav_file:
                frames = wav_file.readframes(-1)
                sample_rate = wav_file.getframerate()
                channels = wav_file.getnchannels()
                sample_width = wav_file.getsampwidth()
                
                # 转换为numpy数组
                if sample_width == 2:
                    audio_data = np.frombuffer(frames, dtype=np.int16).astype(np.float32)
                    audio_data = audio_data / 32768.0
                else:
                    logger.warning(f"不支持的采样位深: {sample_width}")
                    return None
                
                # 处理多声道
                if channels > 1:
                    audio_data = audio_data.reshape(-1, channels)
                    audio_data = np.mean(audio_data, axis=1)
                
                return audio_data
                
        except Exception as e:
            logger.error(f"转换WAV到numpy失败: {e}")
            return None

    async def handle_meeting_summary(self, request: web_request.Request) -> Response:
        """处理会议纪要生成请求"""
        try:
            if not self.llm_service:
                return web.json_response({
                    "success": False,
                    "message": "LLM服务未初始化"
                }, status=503)
            
            data = await request.json()
            text = data.get('text', '')
            template_name = data.get('template', 'default')
            custom_variables = data.get('variables', {})
            
            if not text.strip():
                return web.json_response({
                    "success": False,
                    "message": "文本内容不能为空"
                }, status=400)
            
            # 生成会议纪要
            result = await self.llm_service.generate_meeting_summary(
                text, template_name, custom_variables
            )
            
            # 检查是否生成失败
            if result.get('generation_failed', False):
                return web.json_response({
                    "success": False,
                    "message": f"会议纪要生成失败: {result.get('failure_reason', '未知原因')}",
                    "error_details": result.get('error', ''),
                    "data": result
                }, status=422)  # 使用422表示处理失败但请求格式正确
            
            return web.json_response({
                "success": True,
                "data": result
            })
            
        except Exception as e:
            logger.error(f"会议纪要生成失败: {e}")
            return web.json_response({
                "success": False,
                "message": f"会议纪要生成失败: {str(e)}"
            }, status=500)
    
    async def handle_text_correction(self, request: web_request.Request) -> Response:
        """处理文本纠错请求"""
        try:
            if not self.llm_service:
                return web.json_response({
                    "success": False,
                    "message": "LLM服务未初始化"
                }, status=503)
            
            data = await request.json()
            text = data.get('text', '')
            context = data.get('context', '')
            enable_speaker = data.get('enable_speaker_identification', False)
            
            if not text.strip():
                return web.json_response({
                    "success": False,
                    "message": "文本内容不能为空"
                }, status=400)
            
            # 处理ASR结果
            result = await self.llm_service.process_asr_result(
                text, 
                enable_correction=True,
                enable_speaker_identification=enable_speaker,
                context=context
            )
            
            return web.json_response({
                "success": True,
                "data": result
            })
            
        except Exception as e:
            logger.error(f"文本纠错失败: {e}")
            return web.json_response({
                "success": False,
                "message": f"文本纠错失败: {str(e)}"
            }, status=500)
    
    async def handle_get_templates(self, request: web_request.Request) -> Response:
        """获取可用的模板列表"""
        try:
            if not self.llm_service:
                return web.json_response({
                    "success": False,
                    "message": "LLM服务未初始化"
                }, status=503)
            
            templates = self.llm_service.get_available_templates()
            
            return web.json_response({
                "success": True,
                "data": {
                    "templates": templates,
                    "default_template": CONFIG.llm.default_template
                }
            })
            
        except Exception as e:
            logger.error(f"获取模板列表失败: {e}")
            return web.json_response({
                "success": False,
                "message": f"获取模板列表失败: {str(e)}"
            }, status=500)
    
    async def handle_audio_to_summary(self, request: web_request.Request) -> Response:
        """处理音频文件直接生成会议纪要的请求"""
        import time
        start_time = time.time()
        
        try:
            if not self.llm_service:
                return web.json_response({
                    "success": False,
                    "message": "LLM服务未初始化"
                }, status=503)
            
            logger.info("收到音频转会议纪要请求，预计处理时间可能较长，请耐心等待...")
            
            # 检查Content-Type
            if not request.content_type.startswith('multipart/form-data'):
                return web.json_response({
                    "success": False,
                    "message": "请使用multipart/form-data上传文件"
                }, status=400)
            
            # 读取multipart数据
            reader = await request.multipart()
            
            file_data = None
            filename = None
            template_name = "default"
            enable_correction = True
            enable_speaker = False
            custom_variables = {}
            language = "auto"
            use_itn = True
            callback_url = None
            
            async for field in reader:
                if field.name == 'audio_file':
                    filename = field.filename or "uploaded_file"
                    file_data = await field.read()
                    logger.info(f"接收音频文件: {filename}, 大小: {len(file_data)} bytes")
                    
                    # 保存到临时文件
                    temp_file_path = await temp_file_manager.save_temp_file(
                        file_data, filename
                    )
                elif field.name == 'template_name':
                    template_name = (await field.read()).decode('utf-8')
                elif field.name == 'enable_correction':
                    enable_correction = (await field.read()).decode('utf-8').lower() in ['true', '1', 'yes']
                elif field.name == 'enable_speaker':
                    enable_speaker = (await field.read()).decode('utf-8').lower() in ['true', '1', 'yes']
                elif field.name == 'language':
                    language = (await field.read()).decode('utf-8')
                elif field.name == 'use_itn':
                    use_itn = (await field.read()).decode('utf-8').lower() in ['true', '1', 'yes']
                elif field.name == 'callback':
                    callback_url = (await field.read()).decode('utf-8').strip()
                elif field.name == 'variables':
                    variables_str = (await field.read()).decode('utf-8')
                    try:
                        custom_variables = json.loads(variables_str)
                    except json.JSONDecodeError:
                        logger.warning("自定义变量JSON解析失败")
            
            if 'temp_file_path' not in locals():
                return web.json_response({
                    "success": False,
                    "message": "未收到音频文件"
                }, status=400)
            
            # 验证callback URL格式（如果提供了的话）
            if callback_url and not self._is_valid_url(callback_url):
                return web.json_response({
                    "success": False,
                    "message": "callback URL格式不正确，请提供有效的HTTP/HTTPS URL"
                }, status=400)
            
            # 如果有callback，创建异步任务并立即返回task_id
            if callback_url:
                task_id = task_service.create_task("audio_to_summary", callback_url)
                logger.info(f"创建异步任务: {task_id}, 回调地址: {callback_url}")
                
                # 启动异步任务
                await task_service.execute_async_task(
                    task_id, 
                    self._process_audio_to_summary_async,
                    temp_file_path, filename, template_name, enable_correction, 
                    enable_speaker, custom_variables, language, use_itn
                )
                
                task_info = task_service.get_task_status(task_id)
                return web.json_response({
                    "success": True,
                    "message": "任务已创建，将通过回调返回结果",
                    "data": {
                        "task_id": task_id,
                        "callback_url": callback_url,
                        "created_at": task_info["created_at"]
                    }
                })
            
            # 原有的同步处理逻辑
            try:
                # 第一步：语音识别
                logger.info("🎵 第1步：开始语音识别，预计耗时1-2分钟...")
                asr_start_time = time.time()
                
                # 读取临时文件数据
                file_data = await temp_file_manager.read_temp_file(temp_file_path)
                
                # 使用ASR服务进行语音识别
                recognition_result = await self.asr_service.recognize_audio(
                    file_data,
                    language=language,
                    use_itn=use_itn
                )
                
                asr_duration = time.time() - asr_start_time
                logger.info(f"✅ 语音识别完成，耗时: {asr_duration:.1f}秒")
                
                if not recognition_result or not recognition_result.get("text"):
                    return web.json_response({
                        "success": False,
                        "message": "语音识别失败或未识别到内容"
                    }, status=500)
                
                asr_text = recognition_result["text"]
                logger.info(f"📝 识别文本长度: {len(asr_text)} 字符")
                
                # 第二步：使用LLM处理并生成会议纪要
                logger.info("🤖 第2步：开始LLM处理生成会议纪要，预计耗时2-5分钟...")
                llm_start_time = time.time()
                
                summary_result = await self.llm_service.process_audio_to_summary(
                    asr_text,
                    template_name=template_name,
                    enable_correction=enable_correction,
                    enable_speaker_identification=enable_speaker,
                    custom_variables=custom_variables
                )
                
                llm_duration = time.time() - llm_start_time
                total_duration = time.time() - start_time
                
                logger.info(f"✅ LLM处理完成，耗时: {llm_duration:.1f}秒")
                logger.info(f"🎉 总处理时间: {total_duration:.1f}秒")
                
                # 检查LLM处理是否包含失败情况
                llm_summary = summary_result.get('summary', {})
                if llm_summary.get('generation_failed', False):
                    # 会议纪要生成失败，但语音识别成功
                    logger.warning(f"⚠️ 会议纪要生成失败: {llm_summary.get('failure_reason', '未知原因')}")
                    
                    # 部分成功的结果 - ASR成功，但LLM纪要生成失败
                    partial_result = {
                        "asr_result": recognition_result,
                        "llm_processing": summary_result,
                        "processing_info": {
                            "filename": filename,
                            "file_size": len(file_data),
                            "template_used": template_name,
                            "correction_enabled": enable_correction,
                            "speaker_identification_enabled": enable_speaker,
                            "processing_time": {
                                "asr_duration": round(asr_duration, 1),
                                "llm_duration": round(llm_duration, 1),
                                "total_duration": round(total_duration, 1)
                            }
                        }
                    }
                    
                    return web.json_response({
                        "success": False,
                        "message": f"音频识别成功，但会议纪要生成失败: {llm_summary.get('failure_reason', '大模型输出异常')}",
                        "partial_success": True,  # 标记为部分成功
                        "error_details": llm_summary.get('error', ''),
                        "data": partial_result
                    }, status=422)  # 使用422表示处理失败但请求格式正确
                
                # 完全成功的情况
                final_result = {
                    "asr_result": recognition_result,
                    "llm_processing": summary_result,
                    "processing_info": {
                        "filename": filename,
                        "file_size": len(file_data),
                        "template_used": template_name,
                        "correction_enabled": enable_correction,
                        "speaker_identification_enabled": enable_speaker,
                        "processing_time": {
                            "asr_duration": round(asr_duration, 1),
                            "llm_duration": round(llm_duration, 1),
                            "total_duration": round(total_duration, 1)
                        }
                    }
                }
                
                logger.info("🎯 音频转会议纪要处理完成")
                
                return web.json_response({
                    "success": True,
                    "data": final_result
                })
                
            except Exception as e:
                logger.error(f"❌ 语音识别失败: {e}")
                return web.json_response({
                    "success": False,
                    "message": f"语音识别失败: {str(e)}"
                }, status=500)
            finally:
                # 清理临时文件
                asyncio.create_task(temp_file_manager.cleanup_temp_file(temp_file_path))
            
        except Exception as e:
            total_duration = time.time() - start_time
            logger.error(f"❌ 音频转会议纪要失败: {e}, 总耗时: {total_duration:.1f}秒")
            
            # 清理临时文件
            if 'temp_file_path' in locals():
                asyncio.create_task(temp_file_manager.cleanup_temp_file(temp_file_path))
            
            return web.json_response({
                "success": False,
                "message": f"处理失败: {str(e)}"
            }, status=500)
    
    async def _process_audio_to_summary_async(self, task_id: str, temp_file_path: Path, filename: str, 
                                            template_name: str, enable_correction: bool, enable_speaker: bool,
                                            custom_variables: Dict, language: str, use_itn: bool):
        """异步处理音频转会议纪要任务"""
        import time
        start_time = time.time()
        
        try:
            logger.info(f"开始异步处理任务: {task_id}")
            
            # 读取临时文件
            file_data = await temp_file_manager.read_temp_file(temp_file_path)
            
            # 第一步：语音识别
            logger.info(f"🎵 任务 {task_id} 第1步：开始语音识别...")
            asr_start_time = time.time()
            
            # 更新任务状态
            task_service.update_task_status(task_id, TaskStatus.ASR_PROCESSING, "正在进行语音识别")
            
            recognition_result = await self.asr_service.recognize_audio(
                file_data,
                language=language,
                use_itn=use_itn
            )
            
            asr_duration = time.time() - asr_start_time
            logger.info(f"✅ 任务 {task_id} 语音识别完成，耗时: {asr_duration:.1f}秒")
            
            if not recognition_result or not recognition_result.get("text"):
                raise Exception("语音识别失败或未识别到内容")
            
            asr_text = recognition_result["text"]
            logger.info(f"📝 任务 {task_id} 识别文本长度: {len(asr_text)} 字符")
            
            # 第二步：使用LLM处理并生成会议纪要
            logger.info(f"🤖 任务 {task_id} 第2步：开始LLM处理生成会议纪要...")
            llm_start_time = time.time()
            
            # 更新任务状态
            task_service.update_task_status(task_id, TaskStatus.LLM_PROCESSING, "正在生成会议纪要")
            
            summary_result = await self.llm_service.process_audio_to_summary(
                asr_text,
                template_name=template_name,
                enable_correction=enable_correction,
                enable_speaker_identification=enable_speaker,
                custom_variables=custom_variables
            )
            
            llm_duration = time.time() - llm_start_time
            total_duration = time.time() - start_time
            
            logger.info(f"✅ 任务 {task_id} LLM处理完成，耗时: {llm_duration:.1f}秒")
            logger.info(f"🎉 任务 {task_id} 总处理时间: {total_duration:.1f}秒")
            
            # 构建结果数据
            llm_summary = summary_result.get('summary', {})
            processing_info = {
                "filename": filename,
                "file_size": len(file_data),
                "template_used": template_name,
                "correction_enabled": enable_correction,
                "speaker_identification_enabled": enable_speaker,
                "processing_time": {
                    "asr_duration": round(asr_duration, 1),
                    "llm_duration": round(llm_duration, 1),
                    "total_duration": round(total_duration, 1)
                }
            }
            
            if llm_summary.get('generation_failed', False):
                # 部分成功的结果 - ASR成功，但LLM纪要生成失败
                result_data = {
                    "success": False,
                    "message": f"音频识别成功，但会议纪要生成失败: {llm_summary.get('failure_reason', '大模型输出异常')}",
                    "partial_success": True,
                    "error_details": llm_summary.get('error', ''),
                    "data": {
                        "asr_result": recognition_result,
                        "llm_processing": summary_result,
                        "processing_info": processing_info
                    }
                }
            else:
                # 完全成功的情况
                result_data = {
                    "success": True,
                    "data": {
                        "asr_result": recognition_result,
                        "llm_processing": summary_result,
                        "processing_info": processing_info
                    }
                }
            
            logger.info(f"🎯 任务 {task_id} 音频转会议纪要处理完成")
            
            return result_data
            
        except Exception as e:
            total_duration = time.time() - start_time
            logger.error(f"❌ 任务 {task_id} 处理失败: {e}, 总耗时: {total_duration:.1f}秒")
            
            # 返回失败结果
            error_result = {
                "success": False,
                "message": f"处理失败: {str(e)}",
                "data": {
                    "processing_info": {
                        "filename": filename,
                        "template_used": template_name,
                        "correction_enabled": enable_correction,
                        "speaker_identification_enabled": enable_speaker,
                        "processing_time": {
                            "total_duration": round(total_duration, 1)
                        }
                    }
                }
            }
            
            raise Exception(error_result["message"])
            
        finally:
            # 清理临时文件
            await temp_file_manager.cleanup_temp_file(temp_file_path)
            logger.info(f"任务 {task_id} 临时文件已清理")
