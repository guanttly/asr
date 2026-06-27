{
  "apifoxCli": "1.0.0",
  "info": {
    "name": "JushaAsr HTTP API",
    "description": "JushaAsr HTTP API 提供了完整的语音识别和智能处理服务。除了基础的语音识别功能，还集成了基于大语言模型(LLM)的智能后处理能力，包括文本纠错、说话人识别和会议纪要自动生成。\n\nAPI 支持多种音频格式的上传和识别，提供基础识别和 VAD 分割识别两种模式，并提供热词功能来提高特定词汇的识别准确率。\n\n新增的LLM功能可以将语音识别结果智能处理为结构化的会议纪要，大大提升了语音内容的可用性。系统会自动清理LLM响应中的思考标签（如<think>...</think>），确保返回干净的结果。\n\n**异步回调功能**：音频转会议纪要接口现在支持异步处理，通过提供callback参数可以实现长时间音频文件的非阻塞处理。当提供callback URL时，接口立即返回任务ID，处理完成后自动调用回调地址返回结果。\n\n注意：对于支持深度思考的模型（如Qwen3），系统已在提示词中添加/nothink来减少思考过程的输出。",
    "version": "1.2.0"
  },
  "servers": [
    {
      "url": "http://localhost:8081",
      "description": "本地开发服务器"
    }
  ],
  "paths": {
    "/api/health": {
      "get": {
        "summary": "健康检查",
        "description": "检查服务状态",
        "tags": ["健康检查"],
        "responses": {
          "200": {
            "description": "服务正常",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "status": {
                      "type": "string",
                      "example": "healthy"
                    },
                    "timestamp": {
                      "type": "string",
                      "format": "date-time",
                      "example": "2025-09-12T10:30:00.000Z"
                    },
                    "service": {
                      "type": "string",
                      "example": "JushaAsr HTTP Server"
                    }
                  }
                }
              }
            }
          }
        }
      }
    },
    "/api/upload": {
      "post": {
        "summary": "文件上传识别（兼容接口）",
        "description": "上传音频文件进行语音识别，可选择是否使用VAD分割",
        "tags": ["语音识别"],
        "requestBody": {
          "content": {
            "multipart/form-data": {
              "schema": {
                "type": "object",
                "properties": {
                  "file": {
                    "type": "string",
                    "format": "binary",
                    "description": "音频文件"
                  },
                  "language": {
                    "type": "string",
                    "default": "auto",
                    "enum": ["auto", "zh", "en", "ja", "ko", "yue"],
                    "description": "语言代码"
                  },
                  "use_itn": {
                    "type": "boolean",
                    "default": true,
                    "description": "是否使用逆文本标准化"
                  },
                  "use_vad_segmentation": {
                    "type": "boolean",
                    "default": false,
                    "description": "是否使用VAD分割"
                  },
                  "hotwords": {
                    "type": "string",
                    "description": "热词列表，用逗号、分号或换行符分隔",
                    "example": "科技,人工智能,机器学习"
                  }
                },
                "required": ["file"]
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "识别成功",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/RecognitionResponse"
                }
              }
            }
          },
          "400": {
            "description": "请求参数错误",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ErrorResponse"
                }
              }
            }
          },
          "500": {
            "description": "服务器内部错误",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ErrorResponse"
                }
              }
            }
          }
        }
      }
    },
    "/api/recognize": {
      "post": {
        "summary": "基础音频识别",
        "description": "进行基础音频识别，返回完整文本",
        "tags": ["语音识别"],
        "requestBody": {
          "content": {
            "multipart/form-data": {
              "schema": {
                "type": "object",
                "properties": {
                  "file": {
                    "type": "string",
                    "format": "binary",
                    "description": "音频文件"
                  },
                  "language": {
                    "type": "string",
                    "default": "auto",
                    "enum": ["auto", "zh", "en", "ja", "ko", "yue"],
                    "description": "语言代码"
                  },
                  "use_itn": {
                    "type": "boolean",
                    "default": true,
                    "description": "是否使用逆文本标准化"
                  },
                  "hotwords": {
                    "type": "string",
                    "description": "热词列表，用逗号、分号或换行符分隔",
                    "example": "科技,人工智能,机器学习"
                  }
                },
                "required": ["file"]
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "识别成功",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/BasicRecognitionResponse"
                }
              }
            }
          },
          "400": {
            "description": "请求参数错误",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ErrorResponse"
                }
              }
            }
          },
          "500": {
            "description": "服务器内部错误",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ErrorResponse"
                }
              }
            }
          }
        }
      }
    },
    "/api/recognize/vad": {
      "post": {
        "summary": "VAD分割音频识别",
        "description": "使用VAD（语音活动检测）分割音频后逐段识别，获得更精确的时间戳信息",
        "tags": ["语音识别"],
        "requestBody": {
          "content": {
            "multipart/form-data": {
              "schema": {
                "type": "object",
                "properties": {
                  "file": {
                    "type": "string",
                    "format": "binary",
                    "description": "音频文件"
                  },
                  "language": {
                    "type": "string",
                    "default": "auto",
                    "enum": ["auto", "zh", "en", "ja", "ko", "yue"],
                    "description": "语言代码"
                  },
                  "use_itn": {
                    "type": "boolean",
                    "default": true,
                    "description": "是否使用逆文本标准化"
                  },
                  "hotwords": {
                    "type": "string",
                    "description": "热词列表，用逗号、分号或换行符分隔",
                    "example": "会议,议题,讨论"
                  },
                  "min_segment_duration": {
                    "type": "number",
                    "format": "float",
                    "default": 0.5,
                    "description": "最小语音段时长（秒）"
                  },
                  "max_segment_duration": {
                    "type": "number",
                    "format": "float",
                    "default": 30.0,
                    "description": "最大语音段时长（秒）"
                  }
                },
                "required": ["file"]
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "识别成功",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/VadRecognitionResponse"
                }
              }
            }
          },
          "400": {
            "description": "请求参数错误",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ErrorResponse"
                }
              }
            }
          },
          "500": {
            "description": "服务器内部错误",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ErrorResponse"
                }
              }
            }
          }
        }
      }
    },
    "/api/text/correct": {
      "post": {
        "summary": "文本纠错",
        "description": "使用LLM对语音识别文本进行智能纠错。系统会自动清理LLM响应中的思考标签（如<think>...</think>），确保返回干净的纠错结果。对于支持深度思考的模型（如Qwen3），可在提示词末尾添加/nothink来禁用思考过程。",
        "tags": ["LLM服务"],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "text": {
                    "type": "string",
                    "description": "需要纠错的文本",
                    "example": "今天的会意很重要，我们需要讨论关于产品的各种问提。"
                  },
                  "context": {
                    "type": "string",
                    "description": "上下文信息（可选）",
                    "example": "这是一个项目会议的内容"
                  },
                  "enable_speaker_identification": {
                    "type": "boolean",
                    "description": "是否启用说话人识别（可选，默认false）",
                    "example": false,
                    "default": false
                  }
                },
                "required": ["text"]
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "纠错成功",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/TextCorrectionResponse"
                }
              }
            }
          },
          "400": {
            "description": "请求参数错误",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ErrorResponse"
                }
              }
            }
          },
          "500": {
            "description": "服务器内部错误",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ErrorResponse"
                }
              }
            }
          }
        }
      }
    },
    "/api/meeting/summary": {
      "post": {
        "summary": "生成会议纪要",
        "description": "根据会议对话内容生成结构化的会议纪要。系统会自动清理LLM响应中的思考标签，确保生成干净的会议纪要内容。支持多种专业模板。",
        "tags": ["LLM服务"],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "text": {
                    "type": "string",
                    "description": "会议对话内容",
                    "example": "张三：大家好，今天我们召开这次会议主要是讨论项目进度。李四：我来汇报一下开发进度..."
                  },
                  "template_name": {
                    "type": "string",
                    "description": "会议纪要模板名称",
                    "enum": ["default", "technical_review", "project_progress"],
                    "example": "default"
                  },
                  "enable_correction": {
                    "type": "boolean",
                    "description": "是否启用文本纠错",
                    "example": true
                  },
                  "enable_speaker_identification": {
                    "type": "boolean",
                    "description": "是否启用说话人识别",
                    "example": false
                  },
                  "custom_variables": {
                    "type": "object",
                    "description": "自定义模板变量",
                    "example": {
                      "project_name": "语音识别系统",
                      "project_manager": "张三"
                    }
                  }
                },
                "required": ["text"]
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "纪要生成成功",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/MeetingSummaryResponse"
                }
              }
            }
          },
          "400": {
            "description": "请求参数错误",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ErrorResponse"
                }
              }
            }
          },
          "500": {
            "description": "服务器内部错误",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ErrorResponse"
                }
              }
            }
          }
        }
      }
    },
    "/api/templates": {
      "get": {
        "summary": "获取可用模板列表",
        "description": "获取所有可用的会议纪要模板",
        "tags": ["LLM服务"],
        "responses": {
          "200": {
            "description": "获取成功",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/TemplatesResponse"
                }
              }
            }
          },
          "500": {
            "description": "服务器内部错误",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ErrorResponse"
                }
              }
            }
          }
        }
      }
    },
    "/api/audio/to_summary": {
      "post": {
        "summary": "音频转会议纪要",
        "description": "一键将音频文件转换为结构化会议纪要（ASR + LLM处理）。完整流程包括语音识别、文本纠错、说话人识别（可选）和会议纪要生成。\n\n**同步模式**：不提供callback参数时，接口会等待处理完成后返回完整结果。\n\n**异步模式**：提供callback参数时，接口立即返回任务ID，处理完成后自动调用callback URL返回结果。\n\n系统会自动清理LLM输出中的思考标签。",
        "tags": ["LLM服务"],
        "requestBody": {
          "required": true,
          "content": {
            "multipart/form-data": {
              "schema": {
                "type": "object",
                "properties": {
                  "audio_file": {
                    "type": "string",
                    "format": "binary",
                    "description": "音频文件"
                  },
                  "callback": {
                    "type": "string",
                    "format": "uri",
                    "description": "回调URL，提供此参数时启用异步模式。必须是有效的HTTP/HTTPS URL格式。",
                    "example": "http://your-server.com/callback"
                  },
                  "template_name": {
                    "type": "string",
                    "description": "会议纪要模板名称",
                    "enum": ["default", "technical_review", "project_progress"],
                    "example": "default"
                  },
                  "enable_correction": {
                    "type": "string",
                    "description": "是否启用文本纠错",
                    "enum": ["true", "false"],
                    "example": "true"
                  },
                  "enable_speaker": {
                    "type": "string",
                    "description": "是否启用说话人识别",
                    "enum": ["true", "false"],
                    "example": "false"
                  },
                  "language": {
                    "type": "string",
                    "description": "识别语言",
                    "enum": ["auto", "zh", "en", "yue", "ja", "ko"],
                    "example": "auto"
                  },
                  "use_itn": {
                    "type": "string",
                    "description": "是否使用ITN（逆文本标准化）",
                    "enum": ["true", "false"],
                    "example": "true"
                  },
                  "variables": {
                    "type": "string",
                    "description": "自定义模板变量，JSON格式字符串",
                    "example": "{\"company\": \"ABC公司\", \"meeting_type\": \"周会\"}"
                  }
                },
                "required": ["audio_file"]
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "同步模式：处理成功；异步模式：任务创建成功",
            "content": {
              "application/json": {
                "schema": {
                  "oneOf": [
                    {
                      "$ref": "#/components/schemas/AudioToSummaryResponse"
                    },
                    {
                      "$ref": "#/components/schemas/AsyncTaskResponse"
                    }
                  ]
                }
              }
            }
          },
          "400": {
            "description": "请求参数错误或callback URL格式不正确",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ErrorResponse"
                }
              }
            }
          },
          "422": {
            "description": "部分成功：语音识别成功但会议纪要生成失败",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/PartialSuccessResponse"
                }
              }
            }
          },
          "500": {
            "description": "服务器内部错误",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ErrorResponse"
                }
              }
            }
          },
          "503": {
            "description": "LLM服务未初始化",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ErrorResponse"
                }
              }
            }
          }
        }
      }
    },
    "/api/task/{task_id}": {
      "get": {
        "summary": "查询任务状态",
        "description": "查询异步任务的处理状态和进度信息",
        "tags": ["任务管理"],
        "parameters": [
          {
            "name": "task_id",
            "in": "path",
            "required": true,
            "description": "任务ID",
            "schema": {
              "type": "string",
              "format": "uuid",
              "example": "550e8400-e29b-41d4-a716-446655440000"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "查询成功",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/TaskStatusResponse"
                }
              }
            }
          },
          "404": {
            "description": "任务不存在或已完成",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ErrorResponse"
                }
              }
            }
          },
          "500": {
            "description": "服务器内部错误",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/ErrorResponse"
                }
              }
            }
          }
        }
      }
    }
  },
  "components": {
    "schemas": {
      "RecognitionResponse": {
        "type": "object",
        "properties": {
          "success": {
            "type": "boolean",
            "example": true
          },
          "filename": {
            "type": "string",
            "example": "test.wav"
          },
          "language": {
            "type": "string",
            "example": "auto"
          },
          "use_vad_segmentation": {
            "type": "boolean",
            "example": false
          },
          "result": {
            "type": "object",
            "properties": {
              "success": {
                "type": "boolean",
                "example": true
              },
              "text": {
                "type": "string",
                "example": "你好，这是一个测试音频。"
              },
              "language": {
                "type": "string",
                "example": "zh"
              },
              "confidence": {
                "type": "number",
                "format": "float",
                "example": 0.95
              },
              "duration": {
                "type": "number",
                "format": "float",
                "example": 3.2
              }
            }
          },
          "timestamp": {
            "type": "string",
            "format": "date-time",
            "example": "2025-09-12T10:30:00.000Z"
          }
        }
      },
      "BasicRecognitionResponse": {
        "type": "object",
        "properties": {
          "success": {
            "type": "boolean",
            "example": true
          },
          "filename": {
            "type": "string",
            "example": "meeting.wav"
          },
          "language": {
            "type": "string",
            "example": "auto"
          },
          "use_vad_segmentation": {
            "type": "boolean",
            "example": false
          },
          "result": {
            "type": "object",
            "properties": {
              "success": {
                "type": "boolean",
                "example": true
              },
              "text": {
                "type": "string",
                "example": "欢迎来到人工智能技术分享会，今天我们将讨论机器学习在科技领域的应用。"
              },
              "language": {
                "type": "string",
                "example": "zh"
              },
              "confidence": {
                "type": "number",
                "format": "float",
                "example": 0.92
              },
              "duration": {
                "type": "number",
                "format": "float",
                "example": 8.5
              },
              "method": {
                "type": "string",
                "example": "basic"
              }
            }
          },
          "timestamp": {
            "type": "string",
            "format": "date-time",
            "example": "2025-09-12T10:30:00.000Z"
          }
        }
      },
      "VadRecognitionResponse": {
        "type": "object",
        "properties": {
          "success": {
            "type": "boolean",
            "example": true
          },
          "filename": {
            "type": "string",
            "example": "conversation.wav"
          },
          "language": {
            "type": "string",
            "example": "auto"
          },
          "use_vad_segmentation": {
            "type": "boolean",
            "example": true
          },
          "result": {
            "type": "object",
            "properties": {
              "success": {
                "type": "boolean",
                "example": true
              },
              "text": {
                "type": "string",
                "example": "大家好，欢迎参加今天的会议。我们今天要讨论的主要议题是人工智能技术的发展趋势。"
              },
              "language": {
                "type": "string",
                "example": "zh"
              },
              "method": {
                "type": "string",
                "example": "vad_segmentation"
              },
              "total_segments": {
                "type": "integer",
                "example": 3
              },
              "recognized_segments": {
                "type": "integer",
                "example": 3
              },
              "segments": {
                "type": "array",
                "items": {
                  "type": "object",
                  "properties": {
                    "start_time": {
                      "type": "number",
                      "format": "float",
                      "example": 0.5
                    },
                    "end_time": {
                      "type": "number",
                      "format": "float",
                      "example": 2.1
                    },
                    "text": {
                      "type": "string",
                      "example": "大家好，欢迎参加今天的会议。"
                    },
                    "confidence": {
                      "type": "number",
                      "format": "float",
                      "example": 0.94
                    }
                  }
                }
              },
              "vad_segments": {
                "type": "array",
                "items": {
                  "type": "object",
                  "properties": {
                    "start": {
                      "type": "number",
                      "format": "float",
                      "example": 0.5
                    },
                    "end": {
                      "type": "number",
                      "format": "float",
                      "example": 2.1
                    },
                    "duration": {
                      "type": "number",
                      "format": "float",
                      "example": 1.6
                    }
                  }
                }
              },
              "duration": {
                "type": "number",
                "format": "float",
                "example": 7.5
              },
              "confidence": {
                "type": "number",
                "format": "float",
                "example": 0.92
              }
            }
          },
          "timestamp": {
            "type": "string",
            "format": "date-time",
            "example": "2025-09-12T10:30:00.000Z"
          }
        }
      },
      "TextCorrectionResponse": {
        "type": "object",
        "properties": {
          "success": {
            "type": "boolean",
            "example": true
          },
          "data": {
            "type": "object",
            "properties": {
              "original_text": {
                "type": "string",
                "description": "原始输入文本",
                "example": "今天的会意很重要，我们需要讨论关于产品的各种问提。"
              },
              "corrected_text": {
                "type": "string",
                "description": "纠错后的文本，已自动清理LLM思考标签",
                "example": "今天的会议很重要，我们需要讨论关于产品的各种问题。"
              },
              "speaker_annotated_text": {
                "type": "string",
                "description": "如果启用说话人识别，此字段包含标注结果",
                "example": "今天的会议很重要，我们需要讨论关于产品的各种问题。"
              }
            }
          },
          "timestamp": {
            "type": "string",
            "format": "date-time",
            "example": "2025-09-16T10:30:00.000Z"
          }
        }
      },
      "MeetingSummaryResponse": {
        "type": "object",
        "properties": {
          "success": {
            "type": "boolean",
            "example": true
          },
          "data": {
            "type": "object",
            "properties": {
              "summary": {
                "type": "string",
                "example": "# 会议纪要\n\n## 会议基本信息\n- 会议时间: 2025年9月16日\n- 参与人员: 张三、李四、王五\n\n## 主要议题\n项目进度讨论...\n\n## 决议事项\n1. 加快开发进度\n2. 下周五进行内部测试"
              },
              "template_used": {
                "type": "string",
                "example": "default"
              },
              "variables": {
                "type": "object",
                "example": {
                  "meeting_time": "2025年9月16日",
                  "participants": "张三、李四、王五",
                  "main_discussions": "项目进度讨论"
                }
              },
              "raw_analysis": {
                "type": "string",
                "example": "会议主要讨论了项目进度，参与人员包括张三、李四、王五..."
              }
            }
          },
          "timestamp": {
            "type": "string",
            "format": "date-time",
            "example": "2025-09-16T10:30:00.000Z"
          }
        }
      },
      "TemplatesResponse": {
        "type": "object",
        "properties": {
          "success": {
            "type": "boolean",
            "example": true
          },
          "data": {
            "type": "array",
            "items": {
              "type": "object",
              "properties": {
                "name": {
                  "type": "string",
                  "example": "default"
                },
                "display_name": {
                  "type": "string",
                  "example": "标准会议纪要"
                },
                "description": {
                  "type": "string",
                  "example": "适用于一般会议的标准模板"
                },
                "variables": {
                  "type": "array",
                  "items": {
                    "type": "string"
                  },
                  "example": ["meeting_time", "participants", "main_discussions", "decisions"]
                }
              }
            }
          },
          "timestamp": {
            "type": "string",
            "format": "date-time",
            "example": "2025-09-16T10:30:00.000Z"
          }
        }
      },
      "AudioToSummaryResponse": {
        "type": "object",
        "properties": {
          "success": {
            "type": "boolean",
            "example": true
          },
          "data": {
            "type": "object",
            "properties": {
              "asr_result": {
                "type": "object",
                "properties": {
                  "text": {
                    "type": "string",
                    "example": "张三：大家好，今天我们召开这次会议主要是讨论项目进度。李四：我来汇报一下开发进度..."
                  },
                  "language": {
                    "type": "string",
                    "example": "zh"
                  },
                  "duration": {
                    "type": "number",
                    "format": "float",
                    "example": 120.5
                  },
                  "confidence": {
                    "type": "number",
                    "format": "float",
                    "example": 0.92
                  }
                }
              },
              "llm_processing": {
                "type": "object",
                "properties": {
                  "asr_processing": {
                    "type": "object",
                    "properties": {
                      "original_text": {
                        "type": "string"
                      },
                      "corrected_text": {
                        "type": "string"
                      },
                      "speaker_annotated_text": {
                        "type": "string"
                      }
                    }
                  },
                  "summary": {
                    "type": "object",
                    "properties": {
                      "summary": {
                        "type": "string",
                        "example": "# 会议纪要\n\n## 会议基本信息\n- 会议时间: 2025年9月16日\n..."
                      },
                      "template_used": {
                        "type": "string",
                        "example": "default"
                      }
                    }
                  }
                }
              },
              "processing_info": {
                "type": "object",
                "properties": {
                  "filename": {
                    "type": "string",
                    "example": "meeting_audio.wav"
                  },
                  "file_size": {
                    "type": "integer",
                    "example": 1048576
                  },
                  "template_used": {
                    "type": "string",
                    "example": "default"
                  },
                  "correction_enabled": {
                    "type": "boolean",
                    "example": true
                  },
                  "speaker_identification_enabled": {
                    "type": "boolean",
                    "example": false
                  }
                }
              }
            }
          },
          "timestamp": {
            "type": "string",
            "format": "date-time",
            "example": "2025-09-16T10:30:00.000Z"
          }
        }
      },
      "AsyncTaskResponse": {
        "type": "object",
        "description": "异步任务创建成功的响应",
        "properties": {
          "success": {
            "type": "boolean",
            "example": true
          },
          "message": {
            "type": "string",
            "example": "任务已创建，将通过回调返回结果"
          },
          "data": {
            "type": "object",
            "properties": {
              "task_id": {
                "type": "string",
                "format": "uuid",
                "description": "任务唯一标识符",
                "example": "550e8400-e29b-41d4-a716-446655440000"
              },
              "callback_url": {
                "type": "string",
                "format": "uri",
                "description": "回调地址",
                "example": "http://your-server.com/callback"
              },
              "created_at": {
                "type": "string",
                "format": "date-time",
                "description": "任务创建时间",
                "example": "2025-09-17T10:30:00.123456"
              }
            }
          }
        }
      },
      "TaskStatusResponse": {
        "type": "object",
        "description": "任务状态查询响应",
        "properties": {
          "success": {
            "type": "boolean",
            "example": true
          },
          "data": {
            "type": "object",
            "properties": {
              "task_id": {
                "type": "string",
                "format": "uuid",
                "example": "550e8400-e29b-41d4-a716-446655440000"
              },
              "status": {
                "type": "string",
                "enum": ["processing", "asr_processing", "llm_processing"],
                "description": "任务状态：processing(处理中), asr_processing(语音识别中), llm_processing(生成会议纪要中)",
                "example": "processing"
              },
              "created_at": {
                "type": "string",
                "format": "date-time",
                "description": "任务创建时间",
                "example": "2025-09-17T10:30:00.123456"
              },
              "message": {
                "type": "string",
                "description": "状态描述信息",
                "example": "任务处理中"
              }
            }
          }
        }
      },
      "PartialSuccessResponse": {
        "type": "object",
        "description": "部分成功响应：语音识别成功但会议纪要生成失败",
        "properties": {
          "success": {
            "type": "boolean",
            "example": false
          },
          "message": {
            "type": "string",
            "example": "音频识别成功，但会议纪要生成失败: 大模型输出异常"
          },
          "partial_success": {
            "type": "boolean",
            "description": "标记为部分成功",
            "example": true
          },
          "error_details": {
            "type": "string",
            "description": "详细错误信息",
            "example": "LLM response parsing failed"
          },
          "data": {
            "type": "object",
            "properties": {
              "asr_result": {
                "type": "object",
                "description": "语音识别结果（成功部分）",
                "properties": {
                  "text": {
                    "type": "string",
                    "example": "张三：大家好，今天我们召开这次会议..."
                  },
                  "language": {
                    "type": "string",
                    "example": "zh"
                  },
                  "duration": {
                    "type": "number",
                    "format": "float",
                    "example": 120.5
                  }
                }
              },
              "llm_processing": {
                "type": "object",
                "description": "LLM处理结果（包含失败信息）",
                "properties": {
                  "summary": {
                    "type": "object",
                    "properties": {
                      "generation_failed": {
                        "type": "boolean",
                        "example": true
                      },
                      "failure_reason": {
                        "type": "string",
                        "example": "大模型输出异常"
                      },
                      "error": {
                        "type": "string",
                        "example": "Timeout waiting for LLM response"
                      }
                    }
                  }
                }
              },
              "processing_info": {
                "$ref": "#/components/schemas/ProcessingInfo"
              }
            }
          }
        }
      },
      "CallbackRequestData": {
        "type": "object",
        "description": "回调请求的数据格式",
        "properties": {
          "task_id": {
            "type": "string",
            "format": "uuid",
            "description": "任务ID",
            "example": "550e8400-e29b-41d4-a716-446655440000"
          },
          "timestamp": {
            "type": "string",
            "format": "date-time",
            "description": "回调发送时间",
            "example": "2025-09-17T10:35:00.123456"
          },
          "result": {
            "description": "处理结果数据，根据处理结果不同会有不同的结构",
            "oneOf": [
              {
                "type": "object",
                "description": "成功结果",
                "properties": {
                  "success": {
                    "type": "boolean",
                    "example": true
                  },
                  "data": {
                    "type": "object",
                    "properties": {
                      "asr_result": {
                        "type": "object",
                        "properties": {
                          "text": {
                            "type": "string",
                            "description": "语音识别文本",
                            "example": "张三：大家好，今天我们召开这次会议主要是讨论项目进度。李四：我来汇报一下开发进度..."
                          },
                          "language": {
                            "type": "string",
                            "example": "zh"
                          },
                          "duration": {
                            "type": "number",
                            "format": "float",
                            "example": 120.5
                          },
                          "confidence": {
                            "type": "number",
                            "format": "float",
                            "example": 0.92
                          }
                        }
                      },
                      "llm_processing": {
                        "type": "object",
                        "properties": {
                          "asr_processing": {
                            "type": "object",
                            "description": "ASR文本处理结果",
                            "properties": {
                              "original_text": {
                                "type": "string",
                                "description": "原始识别文本"
                              },
                              "corrected_text": {
                                "type": "string",
                                "description": "纠错后文本"
                              },
                              "speaker_annotated_text": {
                                "type": "string",
                                "description": "说话人标注文本（如启用）"
                              }
                            }
                          },
                          "summary": {
                            "type": "object",
                            "properties": {
                              "content": {
                                "type": "string",
                                "description": "生成的会议纪要内容",
                                "example": "# 会议纪要\n\n## 会议基本信息\n- 会议时间: 2025年9月17日\n- 参与人员: 张三、李四\n\n## 主要议题\n1. 项目进度讨论\n\n## 决议事项\n1. 加快开发进度\n2. 下周进行测试"
                              },
                              "template_used": {
                                "type": "string",
                                "example": "default"
                              },
                              "generation_failed": {
                                "type": "boolean",
                                "example": false
                              },
                              "variables": {
                                "type": "object",
                                "description": "提取的模板变量"
                              }
                            }
                          }
                        }
                      },
                      "processing_info": {
                        "$ref": "#/components/schemas/ProcessingInfo"
                      }
                    }
                  }
                }
              },
              {
                "type": "object",
                "description": "失败结果",
                "properties": {
                  "success": {
                    "type": "boolean",
                    "example": false
                  },
                  "message": {
                    "type": "string",
                    "description": "失败原因",
                    "example": "语音识别失败: 音频格式不支持"
                  },
                  "data": {
                    "type": "object",
                    "properties": {
                      "processing_info": {
                        "$ref": "#/components/schemas/ProcessingInfo"
                      }
                    }
                  }
                }
              },
              {
                "type": "object",
                "description": "部分成功结果（ASR成功，LLM失败）",
                "properties": {
                  "success": {
                    "type": "boolean",
                    "example": false
                  },
                  "message": {
                    "type": "string",
                    "example": "音频识别成功，但会议纪要生成失败: 大模型输出异常"
                  },
                  "partial_success": {
                    "type": "boolean",
                    "example": true
                  },
                  "error_details": {
                    "type": "string",
                    "example": "LLM response parsing failed"
                  },
                  "data": {
                    "type": "object",
                    "properties": {
                      "asr_result": {
                        "type": "object",
                        "description": "成功的语音识别结果",
                        "properties": {
                          "text": {
                            "type": "string",
                            "example": "张三：大家好，今天我们召开这次会议..."
                          },
                          "language": {
                            "type": "string",
                            "example": "zh"
                          },
                          "duration": {
                            "type": "number",
                            "format": "float",
                            "example": 120.5
                          }
                        }
                      },
                      "llm_processing": {
                        "type": "object",
                        "description": "失败的LLM处理结果",
                        "properties": {
                          "summary": {
                            "type": "object",
                            "properties": {
                              "generation_failed": {
                                "type": "boolean",
                                "example": true
                              },
                              "failure_reason": {
                                "type": "string",
                                "example": "大模型输出异常"
                              }
                            }
                          }
                        }
                      },
                      "processing_info": {
                        "$ref": "#/components/schemas/ProcessingInfo"
                      }
                    }
                  }
                }
              }
            ]
          }
        }
      },
      "CallbackSuccessResult": {
        "type": "object",
        "description": "回调成功结果",
        "properties": {
          "task_id": {
            "type": "string",
            "format": "uuid",
            "description": "任务ID",
            "example": "550e8400-e29b-41d4-a716-446655440000"
          },
          "timestamp": {
            "type": "string",
            "format": "date-time",
            "description": "回调发送时间",
            "example": "2025-09-17T10:35:00.123456"
          },
          "result": {
            "type": "object",
            "properties": {
              "success": {
                "type": "boolean",
                "example": true
              },
              "data": {
                "type": "object",
                "properties": {
                  "asr_result": {
                    "type": "object",
                    "properties": {
                      "text": {
                        "type": "string",
                        "description": "语音识别文本",
                        "example": "张三：大家好，今天我们召开这次会议主要是讨论项目进度。李四：我来汇报一下开发进度..."
                      },
                      "language": {
                        "type": "string",
                        "example": "zh"
                      },
                      "duration": {
                        "type": "number",
                        "format": "float",
                        "example": 120.5
                      },
                      "confidence": {
                        "type": "number",
                        "format": "float",
                        "example": 0.92
                      }
                    }
                  },
                  "llm_processing": {
                    "type": "object",
                    "properties": {
                      "asr_processing": {
                        "type": "object",
                        "description": "ASR文本处理结果",
                        "properties": {
                          "original_text": {
                            "type": "string",
                            "description": "原始识别文本"
                          },
                          "corrected_text": {
                            "type": "string",
                            "description": "纠错后文本"
                          },
                          "speaker_annotated_text": {
                            "type": "string",
                            "description": "说话人标注文本（如启用）"
                          }
                        }
                      },
                      "summary": {
                        "type": "object",
                        "properties": {
                          "content": {
                            "type": "string",
                            "description": "生成的会议纪要内容",
                            "example": "# 会议纪要\n\n## 会议基本信息\n- 会议时间: 2025年9月17日\n- 参与人员: 张三、李四\n\n## 主要议题\n1. 项目进度讨论\n\n## 决议事项\n1. 加快开发进度\n2. 下周进行测试"
                          },
                          "template_used": {
                            "type": "string",
                            "example": "default"
                          },
                          "generation_failed": {
                            "type": "boolean",
                            "example": false
                          },
                          "variables": {
                            "type": "object",
                            "description": "提取的模板变量"
                          }
                        }
                      }
                    }
                  },
                  "processing_info": {
                    "$ref": "#/components/schemas/ProcessingInfo"
                  }
                }
              }
            }
          }
        }
      },
      "CallbackFailureResult": {
        "type": "object",
        "description": "回调失败结果",
        "properties": {
          "task_id": {
            "type": "string",
            "format": "uuid",
            "description": "任务ID",
            "example": "550e8400-e29b-41d4-a716-446655440000"
          },
          "timestamp": {
            "type": "string",
            "format": "date-time",
            "description": "回调发送时间",
            "example": "2025-09-17T10:35:00.123456"
          },
          "result": {
            "type": "object",
            "properties": {
              "success": {
                "type": "boolean",
                "example": false
              },
              "message": {
                "type": "string",
                "description": "失败原因",
                "example": "语音识别失败: 音频格式不支持"
              },
              "data": {
                "type": "object",
                "properties": {
                  "processing_info": {
                    "type": "object",
                    "properties": {
                      "filename": {
                        "type": "string",
                        "example": "meeting.wav"
                      },
                      "file_size": {
                        "type": "integer",
                        "example": 1048576
                      },
                      "template_used": {
                        "type": "string",
                        "example": "default"
                      },
                      "processing_time": {
                        "type": "object",
                        "properties": {
                          "total_duration": {
                            "type": "number",
                            "format": "float",
                            "description": "总处理时间（秒）",
                            "example": 15.3
                          }
                        }
                      }
                    }
                  }
                }
              }
            }
          }
        }
      },
      "CallbackPartialSuccessResult": {
        "type": "object",
        "description": "回调部分成功结果（ASR成功，LLM失败）",
        "properties": {
          "task_id": {
            "type": "string",
            "format": "uuid",
            "description": "任务ID",
            "example": "550e8400-e29b-41d4-a716-446655440000"
          },
          "timestamp": {
            "type": "string",
            "format": "date-time",
            "description": "回调发送时间",
            "example": "2025-09-17T10:35:00.123456"
          },
          "result": {
            "type": "object",
            "properties": {
              "success": {
                "type": "boolean",
                "example": false
              },
              "message": {
                "type": "string",
                "example": "音频识别成功，但会议纪要生成失败: 大模型输出异常"
              },
              "partial_success": {
                "type": "boolean",
                "example": true
              },
              "error_details": {
                "type": "string",
                "example": "LLM response parsing failed"
              },
              "data": {
                "type": "object",
                "properties": {
                  "asr_result": {
                    "type": "object",
                    "description": "成功的语音识别结果",
                    "properties": {
                      "text": {
                        "type": "string",
                        "example": "张三：大家好，今天我们召开这次会议..."
                      },
                      "language": {
                        "type": "string",
                        "example": "zh"
                      },
                      "duration": {
                        "type": "number",
                        "format": "float",
                        "example": 120.5
                      }
                    }
                  },
                  "llm_processing": {
                    "type": "object",
                    "description": "失败的LLM处理结果",
                    "properties": {
                      "summary": {
                        "type": "object",
                        "properties": {
                          "generation_failed": {
                            "type": "boolean",
                            "example": true
                          },
                          "failure_reason": {
                            "type": "string",
                            "example": "大模型输出异常"
                          }
                        }
                      }
                    }
                  },
                  "processing_info": {
                    "$ref": "#/components/schemas/ProcessingInfo"
                  }
                }
              }
            }
          }
        }
      },
      "ProcessingInfo": {
        "type": "object",
        "description": "处理信息",
        "properties": {
          "filename": {
            "type": "string",
            "description": "音频文件名",
            "example": "meeting_audio.wav"
          },
          "file_size": {
            "type": "integer",
            "description": "文件大小（字节）",
            "example": 1048576
          },
          "template_used": {
            "type": "string",
            "description": "使用的模板",
            "example": "default"
          },
          "correction_enabled": {
            "type": "boolean",
            "description": "是否启用了文本纠错",
            "example": true
          },
          "speaker_identification_enabled": {
            "type": "boolean",
            "description": "是否启用了说话人识别",
            "example": false
          },
          "processing_time": {
            "type": "object",
            "description": "处理时间统计",
            "properties": {
              "asr_duration": {
                "type": "number",
                "format": "float",
                "description": "语音识别耗时（秒）",
                "example": 45.2
              },
              "llm_duration": {
                "type": "number",
                "format": "float",
                "description": "LLM处理耗时（秒）",
                "example": 78.3
              },
              "total_duration": {
                "type": "number",
                "format": "float",
                "description": "总处理时间（秒）",
                "example": 123.5
              }
            }
          }
        }
      },
      "ErrorResponse": {
        "type": "object",
        "properties": {
          "success": {
            "type": "boolean",
            "example": false
          },
          "message": {
            "type": "string",
            "example": "错误描述信息"
          },
          "timestamp": {
            "type": "string",
            "format": "date-time",
            "example": "2025-09-12T10:30:00.000Z"
          }
        }
      }
    }
  },
  "tags": [
    {
      "name": "健康检查",
      "description": "服务状态检查相关接口"
    },
    {
      "name": "语音识别", 
      "description": "语音识别相关接口"
    },
    {
      "name": "LLM服务",
      "description": "基于大语言模型的智能处理服务，包括文本纠错、说话人识别和会议纪要生成"
    },
    {
      "name": "任务管理",
      "description": "异步任务管理相关接口，包括任务状态查询和回调功能"
    }
  ]
}
